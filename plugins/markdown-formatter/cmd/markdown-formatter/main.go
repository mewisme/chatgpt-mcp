package main

import (
	"fmt"
	"os"

	"go.mewis.me/chatgpt-mcp/internal/formatter"
	markdownformatter "go.mewis.me/chatgpt-mcp/plugins/markdown-formatter"
)

func main() {
	req, err := formatter.DecodeRequest(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	text, err := markdownformatter.Render(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := formatter.EncodeResponse(os.Stdout, formatter.Response{Text: text}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
