package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	cftunnel "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtimeplugin.Serve(ctx, os.Stdin, os.Stdout, cftunnel.NewHandler()); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
