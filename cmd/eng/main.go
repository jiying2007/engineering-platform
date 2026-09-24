package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/routing"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "api":
		if err := remoteAPI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "capabilities":
		printJSON(map[string]any{
			"capabilities": embedded.Capabilities(),
			"skills":       embedded.Skills(),
		})
	case "route":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: eng route <task-type> <subsystem>")
			os.Exit(2)
		}
		route, err := routing.Resolve(os.Args[2], os.Args[3])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		printJSON(route)
	case "material-check":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: eng material-check <manifest.json>")
			os.Exit(2)
		}
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var manifest material.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		result := material.Evaluate(manifest)
		printJSON(result)
		if result.Status == material.Blocked {
			os.Exit(3)
		}
	default:
		usage()
	}
}

func printJSON(value any) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func usage() {
	fmt.Println("eng <command>")
	fmt.Println("  api GET|POST /api/v1/path [body.json]   call authenticated Control API")
	fmt.Println("  capabilities                         list embedded capabilities and skills")
	fmt.Println("  route <task-type> <subsystem>       resolve explicit M1 capability/skill route")
	fmt.Println("  material-check <manifest.json>      evaluate material readiness")
}
