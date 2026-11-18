package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jiying2007/engineering-platform/tool/internal/local"
)

func main() {
	if local.MaybeRunChild() {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	os.Exit(local.CLI(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
