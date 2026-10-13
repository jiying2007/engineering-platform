package main
import("encoding/json";"fmt";"go/ast";"go/importer";"go/parser";"go/token";"go/types";"io";"os";"path/filepath";"sort";"strconv";"strings")
type Package struct { Dir, ImportPath, Name, Export string; GoFiles, CgoFiles []string; ImportMap map[string]string }
type Edit struct { Start, End int; Text string }
type Wrapper struct { Path, Receiver, Name, Key string; Start, End int }
func must(err error) { if err!=nil { panic(err) } }
func normalize(s string) string { return strings.Split(s," [")[0] }
func receiverName(expr ast.Expr) string { if p,ok:=expr.(*ast.StarExpr);ok { expr=p.X }; n,ok:=expr.(*ast.Ident); if !ok{return ""};return n.Name }
func key(o *types.Func) string {
 if o.Pkg()==nil{return ""}
 s,ok:=o.Type().(*types.Signature);if !ok||s.Recv()==nil{return ""}
 t:=s.Recv().Type();if p,ok:=t.(*types.Pointer);ok{t=p.Elem()};n,ok:=t.(*types.Named);if !ok{return ""}
 return normalize(o.Pkg().Path())+"."+n.Obj().Name()+"."+o.Name()
}
func main(){
 root,err:=filepath.Abs(os.Args[1]);must(err)
 input,err:=os.Open(os.Args[2]);must(err);defer input.Close()
 var packages []Package;exports:=map[string]string{};dec:=json.NewDecoder(input)
 for { var p Package;err:=dec.Decode(&p);if err==io.EOF{break};must(err);packages=append(packages,p);if p.Export!=""{exports[p.ImportPath]=p.Export} }
 edits:=map[string]map[Edit]bool{};add:=func(path string,e Edit){if edits[path]==nil{edits[path]=map[Edit]bool{}};edits[path][e]=true}
 wrappers:=map[string]Wrapper{};var list []Wrapper
 for _,p:=range packages{
  if !strings.HasPrefix(p.Dir,root+"/")||strings.Contains(p.ImportPath," [")||strings.HasSuffix(p.ImportPath,".test")||strings.HasPrefix(p.Dir,root+"/vendor/"){continue}
  for _,name:=range p.GoFiles{
   if strings.HasSuffix(name,"_test.go"){continue};path:=name;if !filepath.IsAbs(path){path=filepath.Join(p.Dir,name)}
   fs:=token.NewFileSet();f,err:=parser.ParseFile(fs,path,nil,parser.ParseComments);must(err)
   for _,d:=range f.Decls{
    fn,ok:=d.(*ast.FuncDecl);if !ok||fn.Recv==nil||fn.Body==nil||len(fn.Body.List)!=1{continue}
    ret,ok:=fn.Body.List[0].(*ast.ReturnStmt);if !ok||len(ret.Results)!=1{continue}
    call,ok:=ret.Results[0].(*ast.CallExpr);if !ok||len(call.Args)<1{continue}
    sel,ok:=call.Fun.(*ast.SelectorExpr);if !ok||sel.Sel.Name!=fn.Name.Name+"Context"{continue}
    owner,ok:=sel.X.(*ast.Ident);if !ok||len(fn.Recv.List[0].Names)!=1||owner.Name!=fn.Recv.List[0].Names[0].Name{continue}
    bg,ok:=call.Args[0].(*ast.CallExpr);if !ok||len(bg.Args)!=0{continue};bs,ok:=bg.Fun.(*ast.SelectorExpr);if !ok||bs.Sel.Name!="Background"{continue};pkg,ok:=bs.X.(*ast.Ident);if !ok||pkg.Name!="context"{continue}
    w:=Wrapper{Path:path,Receiver:receiverName(fn.Recv.List[0].Type),Name:fn.Name.Name,Start:fs.Position(fn.Pos()).Offset,End:fs.Position(fn.End()).Offset}
    if fn.Doc!=nil { w.Start=fs.Position(fn.Doc.Pos()).Offset }
    w.Key=normalize(p.ImportPath)+"."+w.Receiver+"."+w.Name
    if _,exists:=wrappers[w.Key];exists{panic("duplicate wrapper "+w.Key)}
    wrappers[w.Key]=w;list=append(list,w);add(path,Edit{w.Start,w.End,""})
   }
  }
 }
 if len(wrappers)!=75{panic(fmt.Sprintf("expected exactly75 wrappers, got%d",len(wrappers)))}
 rewritten:=map[string]bool{};references:=[]string{};seenPackages:=0
 for _,p:=range packages{
  if !strings.HasPrefix(p.Dir,root+"/")||strings.HasPrefix(p.Dir,root+"/vendor/")||strings.HasSuffix(p.ImportPath,".test"){continue}
  seenPackages++
  fs:=token.NewFileSet();var files []*ast.File
  for _,name:=range p.GoFiles {path:=name;if !filepath.IsAbs(path){path=filepath.Join(p.Dir,name)};f,err:=parser.ParseFile(fs,path,nil,0);must(err);files=append(files,f)}
  imp:=importer.ForCompiler(fs,"gc",func(path string)(io.ReadCloser,error){if mapped:=p.ImportMap[path];mapped!=""{path=mapped};out,ok:=exports[path];if !ok{return nil,fmt.Errorf("no export for%s",path)};return os.Open(out)})
  info:=&types.Info{Selections:map[*ast.SelectorExpr]*types.Selection{},Uses:map[*ast.Ident]types.Object{}}
  conf:=types.Config{Importer:imp,Sizes:types.SizesFor("gc","amd64")}
  _,err:=conf.Check(normalize(p.ImportPath),fs,files,info);if err!=nil{panic(fmt.Errorf("%s: %w",p.ImportPath,err))}
  for _,file:=range files {
   path:=fs.Position(file.Pos()).Filename
   parents:=map[ast.Node]ast.Node{};var stack []ast.Node
   ast.Inspect(file,func(n ast.Node)bool{if n==nil{stack=stack[:len(stack)-1];return true};if len(stack)>0{parents[n]=stack[len(stack)-1]};stack=append(stack,n);return true})
   for sel,selection:=range info.Selections{
    if fs.Position(sel.Pos()).Filename!=path{continue};obj,ok:=selection.Obj().(*types.Func);if !ok{continue};w,ok:=wrappers[key(obj)];if !ok{continue}
    if !strings.HasSuffix(path,"_test.go"){panic("production wrapper use "+fs.Position(sel.Pos()).String()+" "+w.Key)}
    call,ok:=parents[sel].(*ast.CallExpr);if !ok||call.Fun!=sel||selection.Kind()!=types.MethodVal{panic("non-call wrapper use "+fs.Position(sel.Pos()).String())}
    add(path,Edit{fs.Position(sel.Sel.Pos()).Offset,fs.Position(sel.Sel.End()).Offset,sel.Sel.Name+"Context"})
    insertion:=fs.Position(call.Lparen).Offset+1;text:="context.Background()";if len(call.Args)>0{text+=", "};add(path,Edit{insertion,insertion,text})
    id:=fs.Position(sel.Pos()).String();if !rewritten[id]{rewritten[id]=true;references=append(references,id+" "+w.Key)}
   }
  }
 }
 var changed []string
 for path,unique:=range edits {
  raw,err:=os.ReadFile(path);must(err);var changes []Edit;for e:=range unique{changes=append(changes,e)}
  sort.Slice(changes,func(i,j int)bool{return changes[i].Start>changes[j].Start})
  last:=len(raw);for _,e:=range changes{if e.End>last{panic("overlapping edits")};raw=append(append(append([]byte{},raw[:e.Start]...),[]byte(e.Text)...),raw[e.End:]...);last=e.Start}
  if strings.HasSuffix(path,"_test.go"){
   fs:=token.NewFileSet();file,err:=parser.ParseFile(fs,path,raw,0);must(err);has:=false
   for _,i:=range file.Imports{v,err:=strconv.Unquote(i.Path.Value);must(err);if v=="context"{if i.Name!=nil&&i.Name.Name!="context"{panic("aliased context import")};has=true}}
   if !has{off:=fs.Position(file.Name.End()).Offset;raw=append(append(append([]byte{},raw[:off]...),[]byte("\n\nimport \"context\"\n")...),raw[off:]...)}
  }
  must(os.WriteFile(path,raw,0644));rel,err:=filepath.Rel(root,path);must(err);changed=append(changed,rel)
 }
 sort.Strings(changed);sort.Strings(references);sort.Slice(list,func(i,j int)bool{return list[i].Key<list[j].Key})
 for i:=range list{list[i].Path,_=filepath.Rel(root,list[i].Path)}
 out:=map[string]any{"removed":list,"production_references":0,"test_references":references,"changed":changed,"type_checked_packages":seenPackages,"base_tree":"567db158c2b3f7ede3ef49a20a74c4d802a2ef8c"}
 enc:=json.NewEncoder(os.Stdout);enc.SetIndent("","  ");must(enc.Encode(out))
}
