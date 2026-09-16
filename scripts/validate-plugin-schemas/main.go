package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type schemaTarget struct {
	Schema string
	Files  []string
}

func main() {
	if err := validatePluginSchemas(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validatePluginSchemas() error {
	manifests, err := filepath.Glob(filepath.Join("plugins", "*", "plugin.json"))
	if err != nil {
		return err
	}
	sort.Strings(manifests)
	if len(manifests) == 0 {
		return fmt.Errorf("no plugin manifests found")
	}
	targets := []schemaTarget{
		{Schema: filepath.Join("plugins", "schema.json"), Files: manifests},
		{Schema: filepath.Join("plugins", "workflow.schema.json"), Files: []string{filepath.Join("plugins", "workflow.json")}},
		{Schema: filepath.Join("plugins", "registry", "index.schema.json"), Files: []string{filepath.Join("plugins", "registry", "index.json")}},
		{Schema: filepath.Join("plugins", "registry", "publishers.schema.json"), Files: []string{filepath.Join("plugins", "registry", "publishers.json")}},
	}
	for _, target := range targets {
		compiler := jsonschema.NewCompiler()
		compiler.AssertFormat()
		schema, err := compiler.Compile(target.Schema)
		if err != nil {
			return fmt.Errorf("compile JSON schema %s: %w", target.Schema, err)
		}
		for _, path := range target.Files {
			value, err := readJSON(path)
			if err != nil {
				return err
			}
			if err := schema.Validate(value); err != nil {
				return fmt.Errorf("validate %s against %s: %w", path, target.Schema, err)
			}
		}
	}
	return nil
}

func readJSON(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return value, nil
}
