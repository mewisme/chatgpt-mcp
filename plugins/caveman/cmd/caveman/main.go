package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	cavemanplugin "go.mewis.me/chatgpt-mcp/plugins/caveman"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtimeplugin.Serve(ctx, os.Stdin, os.Stdout, cavemanplugin.NewHandler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
