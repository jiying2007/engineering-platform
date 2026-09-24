package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jiying2007/engineering-platform/internal/embedded"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "capabilities":
		out := map[string]any{
			"capabilities": embedded.Capabilities(),
			"skills":       embedded.Skills(),
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
	default:
		usage()
	}
}

func usage() {
	fmt.Println("eng <command>")
	fmt.Println("  capabilities   list embedded capabilities and initial skills")
}
