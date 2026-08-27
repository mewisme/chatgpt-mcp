package main

import (
	"os"

	"go.mewis.me/chatgpt-mcp/internal/cli"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func main() {
	if err := cli.Execute(); err != nil {
		logger.NewCLIWithWriter(os.Stderr).Error("CLI", err.Error())
		os.Exit(1)
	}
}
