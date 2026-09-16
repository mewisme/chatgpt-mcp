package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	ponytailplugin "go.mewis.me/chatgpt-mcp/plugins/ponytail"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtimeplugin.Serve(ctx, os.Stdin, os.Stdout, ponytailplugin.NewHandler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
