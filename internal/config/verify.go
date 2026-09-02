package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

type VerifyResult struct {
	Format configformat.Format
	Ext    string
	Files  int
}

func Verify() (VerifyResult, error) {
	return verifyAt(RootPath(), false)
}

func VerifyRuntime() (VerifyResult, error) {
	return verifyAt(RootPath(), true)
}

func verifyAt(root string, skipCheckpoints bool) (VerifyResult, error) {
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
		if skipCheckpoints && isCheckpointStateFile(root, file.path) {
			continue
		}
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

func isCheckpointStateFile(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	return len(parts) >= 4 && parts[0] == "workspaces" && parts[2] == "checkpoints"
}
