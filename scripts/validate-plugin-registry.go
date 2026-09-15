package main

import (
	"fmt"
	"os"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/validate-plugin-registry.go <index.json> <publishers.json>")
		os.Exit(2)
	}
	indexData, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	publishersData, err := os.ReadFile(os.Args[2])
	if err != nil {
		fail(err)
	}
	index, err := plugin.ParseRegistryIndex(indexData)
	if err != nil {
		fail(err)
	}
	publishers, err := plugin.ParsePublisherIndex(publishersData)
	if err != nil {
		fail(err)
	}
	snapshot := plugin.RegistrySnapshot{Registry: plugin.OfficialRegistry(), Index: index, Publishers: publishers}
	if err := snapshot.Validate(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
