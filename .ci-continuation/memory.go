// Temporary exact-preimage materializer; removed before final qualification.
package main

import (
 "bytes"
 "crypto/sha1"
 "fmt"
 "go/ast"
 "go/format"
 "go/parser"
 "go/token"
 "os"
 "sort"
 "strings"
)

type edit struct { start, end int; text string }
func main() {
 path := "internal/store/memory.go"
 source, err := os.ReadFile(path); if err != nil { panic(err) }
 object := append([]byte(fmt.Sprintf("blob %d\x00", len(source))), source...)
 if fmt.Sprintf("%x", sha1.Sum(object)) != "b779e5522ffb441c2c0fd43942c2860a085d374e" { panic("Memory preimage drift") }
 set := token.NewFileSet()
 file, err := parser.ParseFile(set, path, source, parser.ParseComments); if err != nil { panic(err) }
 offset := func(pos token.Pos) int { return set.Position(pos).Offset }
 text := func(n ast.Node) string { return string(source[offset(n.Pos()):offset(n.End())]) }
 methods := map[string]*ast.FuncDecl{}
 for _, decl := range file.Decls {
  fn, ok := decl.(*ast.FuncDecl); if !ok || fn.Recv == nil || len(fn.Recv.List)!=1 { continue }
  pointer, ok := fn.Recv.List[0].Type.(*ast.StarExpr); if !ok { continue }
  typ, ok := pointer.X.(*ast.Ident); if !ok || typ.Name!="Memory" { continue }
  if len(fn.Recv.List[0].Names)!=1 || fn.Recv.List[0].Names[0].Name!="m" { panic("receiver drift") }
  methods[fn.Name.Name]=fn
 }
 changes:=[]edit{};count:=0
 for name, scoped:=range methods {
  if !strings.HasSuffix(name,"Context") { continue }
  baseName:=strings.TrimSuffix(name,"Context");base:=methods[baseName]
  if base==nil || scoped.Body==nil || len(scoped.Body.List)!=2 { panic("scoped delegate drift: "+name) }
  guard,ok:=scoped.Body.List[0].(*ast.IfStmt);if !ok || !strings.Contains(text(guard),"ctx.Err()") {panic("guard drift")}
  ret,ok:=scoped.Body.List[1].(*ast.ReturnStmt);if !ok || len(ret.Results)!=1 {panic("return drift")}
  call,ok:=ret.Results[0].(*ast.CallExpr);if !ok {panic("call drift")}
  sel,ok:=call.Fun.(*ast.SelectorExpr);if !ok || sel.Sel.Name!=baseName || text(sel.X)!="m" {panic("target drift")}
  params:=string(source[offset(base.Type.Params.Opening)+1:offset(base.Type.Params.Closing)])
  args:=[]string{}
  for _,field:=range base.Type.Params.List {for _,arg:=range field.Names {args=append(args,arg.Name)}}
  if len(args)!=len(call.Args){panic("argument drift")}
  parameterList:="ctx context.Context";if strings.TrimSpace(params)!=""{parameterList+=", "+params}
  resultText:="";if base.Type.Results!=nil{resultText=" "+text(base.Type.Results)}
  body:=text(base.Body);lockCount:=0;insertAt:=-1
  for _,stmt:=range base.Body.List {
   if expression,ok:=stmt.(*ast.ExprStmt);ok {if text(expression)=="m.mu.Lock()" || text(expression)=="m.mu.RLock()" {lockCount++}}
   if deferred,ok:=stmt.(*ast.DeferStmt);ok {if text(deferred)=="defer m.mu.Unlock()" || text(deferred)=="defer m.mu.RUnlock()" {insertAt=offset(deferred.End())-offset(base.Body.Pos())}}
  }
  if lockCount!=1 || insertAt<0{panic("original lock drift")}
  guardText:=text(guard)
  body=body[:insertAt]+"\n"+guardText+body[insertAt:]
  body="{\n"+guardText+body[1:]
  newScoped:="func (m *Memory) "+name+"("+parameterList+")"+resultText+" "+body
  bridgeArgs:="context.Background()";if len(args)>0{bridgeArgs+=", "+strings.Join(args,", ")}
  bridge:="{ return m."+name+"("+bridgeArgs+") }"
  newBase:=string(source[offset(base.Pos()):offset(base.Body.Pos())])+bridge
  changes=append(changes,edit{offset(base.Pos()),offset(base.End()),newScoped},edit{offset(scoped.Pos()),offset(scoped.End()),newBase})
  count++
 }
 if count!=32{panic(fmt.Sprintf("expected 32 delegates, got %d",count))}
 sort.Slice(changes,func(i,j int)bool{return changes[i].start>changes[j].start})
 output:=bytes.Clone(source);last:=len(output)
 for _,e:=range changes{if e.end>last{panic("overlapping edit")};output=append(append(append([]byte(nil),output[:e.start]...),[]byte(e.text)...),output[e.end:]...);last=e.start}
 output,err=format.Source(output);if err!=nil{panic(err)}
 if err=os.WriteFile(path,output,0644);err!=nil{panic(err)}
 fmt.Println("32 exact Memory delegates materialized; original business bodies and lock retained")
}
