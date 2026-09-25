package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func main() {
	executable := flag.String("codex", "", "absolute path to the qualified native Codex binary")
	digest := flag.String("digest", "", "qualified raw-byte sha256 digest")
	work := flag.String("work", "", "empty/read-only qualification workspace")
	home := flag.String("home", "", "fresh owner-private Codex HOME")
	rule := flag.String("federation-rule", "", "Codex workload identity federation rule ID")
	token := flag.String("identity-token-file", "", "absolute owner-private identity token file")
	audit := flag.String("audit-context", "", "optional bounded JSON workload attribution")
	model := flag.String("model", "gpt-5.6-sol", "qualified model identifier")
	flag.Parse()
	if flag.NArg() != 0 || *executable == "" || *digest == "" || *work == "" || *home == "" || *rule == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "usage: codex-wif-live --codex <absolute> --digest sha256:<...> --work <dir> --home <fresh-dir> --federation-rule <rule-id> --identity-token-file <absolute> [--audit-context <json>] [--model gpt-5.6-sol]")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	receipt, err := codexapp.LiveWIFProbe(ctx, *executable, *digest, *work, *home, *rule, *token, *audit, *model)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := codexapp.MarshalLiveReceipt(receipt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
