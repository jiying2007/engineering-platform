package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jiying2007/engineering-platform/internal/cievidence"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func main() {
	input:=flag.String("input","","path to a strict CI evidence receipt JSON")
	flag.Parse()
	if *input=="" || flag.NArg()!=0 {
		fmt.Fprintln(os.Stderr,"usage: ci-evidence --input <receipt.json>")
		os.Exit(2)
	}
	data,err:=os.ReadFile(*input)
	if err!=nil { fail(err) }
	var receipt cievidence.Receipt
	if err=strictjson.Decode(data,&receipt); err!=nil { fail(err) }
	cievidence.Sort(&receipt)
	env,err:=cievidence.NewEnvelope(receipt)
	if err!=nil { fail(err) }
	out,err:=json.MarshalIndent(env,"","  ")
	if err!=nil { fail(err) }
	if _,err=os.Stdout.Write(append(out,'\n')); err!=nil { fail(err) }
}
func fail(err error){
	fmt.Fprintln(os.Stderr,err)
	os.Exit(1)
}
