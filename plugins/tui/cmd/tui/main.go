package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	commandtui "go.mewis.me/chatgpt-mcp/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	route, err := commandtui.ParseRoute(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := commandtui.Run(ctx, route, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
