package config

import (
	"errors"
	"fmt"
	"os"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

type VerifyResult struct {
	Format configformat.Format
	Ext    string
	Files  int
}

func Verify() (VerifyResult, error) {
	return verifyAt(RootPath())
}

func verifyAt(root string) (VerifyResult, error) {
	source, err := configformat.Discover(root)
	if err != nil {
		return VerifyResult{}, err
	}
	if !source.Exists {
		return VerifyResult{}, errors.New("configuration is not initialized")
	}
	files, err := collectStructuredFiles(root)
	if err != nil {
		return VerifyResult{}, err
	}
	for _, file := range files {
		if file.ext != source.Ext {
			return VerifyResult{}, fmt.Errorf("structured config format mismatch: %s uses %s, expected %s", file.path, file.ext, source.Ext)
		}
		data, err := os.ReadFile(file.path)
		if err != nil {
			return VerifyResult{}, err
		}
		if _, err := configformat.DecodeGeneric(file.format, data); err != nil {
			return VerifyResult{}, fmt.Errorf("decode %s: %w", file.path, err)
		}
	}
	cfg, err := loadAt(source.Path, configformat.StructuredPathFrom(source.Path, "tunnel"))
	if err != nil {
		return VerifyResult{}, err
	}
	if err := Validate(cfg); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Format: source.Format, Ext: source.Ext, Files: len(files)}, nil
}
