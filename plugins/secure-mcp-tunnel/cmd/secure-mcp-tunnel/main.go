package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	securemcptunnel "go.mewis.me/chatgpt-mcp/plugins/secure-mcp-tunnel"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtimeplugin.Serve(ctx, os.Stdin, os.Stdout, securemcptunnel.NewHandler()); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
