package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/licenseinventory"
)

func main() {
	root, artifactID, output := ".", "", ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			if i >= len(args) {
				fail(fmt.Errorf("--root requires a value"))
			}
			root = args[i]
		case "--artifact":
			i++
			if i >= len(args) {
				fail(fmt.Errorf("--artifact requires a value"))
			}
			artifactID = args[i]
		case "--out":
			i++
			if i >= len(args) {
				fail(fmt.Errorf("--out requires a value"))
			}
			output = args[i]
		default:
			fail(fmt.Errorf("usage: go run ./internal/licenseinventory/cmd/license-inventory --artifact <id|all> [--root .] [--out dir]"))
		}
	}
	if artifactID == "" {
		fail(fmt.Errorf("usage: go run ./internal/licenseinventory/cmd/license-inventory --artifact <id|all> [--root .] [--out dir]"))
	}
	root, err := filepath.Abs(root)
	if err != nil {
		fail(err)
	}
	policy, err := licenseinventory.LoadPolicy(root)
	if err != nil {
		fail(err)
	}
	ids := []string{artifactID}
	if artifactID == "all" {
		ids = nil
		for _, artifact := range licenseinventory.Artifacts() {
			ids = append(ids, artifact.ID)
		}
	}
	collector := licenseinventory.Collector{Root: root}
	for _, id := range ids {
		inv, err := collector.Generate(id)
		if err != nil {
			fail(err)
		}
		if err := licenseinventory.Validate(inv, policy); err != nil {
			fail(fmt.Errorf("%s: %w", id, err))
		}
		dir := output
		if dir == "" {
			dir = filepath.Join(root, "dist", "licenses", id)
		} else if artifactID == "all" {
			dir = filepath.Join(output, id)
		}
		if err := licenseinventory.Write(inv, dir); err != nil {
			fail(err)
		}
		fmt.Printf("artifact=%s packages=%d out=%s\n", id, len(inv.Packages), dir)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
