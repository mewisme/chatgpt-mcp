package main

import (
	"os"

	"go.mewis.me/chatgpt-mcp/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
