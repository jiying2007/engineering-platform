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
	savedLogin := flag.String("saved-login-file", "", "absolute owner-private saved ChatGPT auth.json")
	model := flag.String("model", "gpt-5.6-sol", "qualified model identifier")
	flag.Parse()
	if flag.NArg() != 0 || *executable == "" || *digest == "" || *work == "" || *home == "" || *savedLogin == "" {
		fmt.Fprintln(os.Stderr, "usage: codex-saved-login-live --codex <absolute> --digest sha256:<...> --work <dir> --home <fresh-dir> --saved-login-file <absolute-auth.json> [--model gpt-5.6-sol]")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	receipt, err := codexapp.LiveSavedLoginProbe(ctx, *executable, *digest, *work, *home, *savedLogin, *model)
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
