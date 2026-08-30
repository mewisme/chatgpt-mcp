package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

var structuredStateNames = map[string]bool{
	"config": true, "tunnel": true, "upstream": true, "workspaces": true, "oauth": true,
	"shell": true, "index": true, "manifest": true,
}

type conversionFile struct {
	source string
	target string
	data   []byte
	orig   []byte
	mode   os.FileMode
}

func ConvertFormat(target configformat.Format) (int, error) {
	return convertFormatAt(RootPath(), target)
}

func convertFormatAt(root string, target configformat.Format) (int, error) {
	source, err := configformat.Discover(root)
	if err != nil {
		return 0, err
	}
	if !source.Exists {
		return 0, errors.New("configuration is not initialized")
	}
	targetExt := configformat.Extension(target)
	if source.Format == target && source.Ext == targetExt {
		return 0, nil
	}

	files := make([]conversionFile, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != source.Ext {
			return nil
		}
		base := strings.TrimSuffix(entry.Name(), source.Ext)
		if !structuredStateNames[base] {
			return nil
		}
		original, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		raw, err := configformat.DecodeGeneric(source.Format, original)
		if err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		if base == "upstream" {
			if values, ok := raw.([]any); ok {
				raw = map[string]any{"servers": values}
			}
		}
		encoded, err := configformat.EncodeGeneric(target, raw)
		if err != nil {
			return fmt.Errorf("encode %s as %s: %w", path, target, err)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		targetPath := filepath.Join(filepath.Dir(path), base+targetExt)
		if targetPath != path {
			if _, err := os.Stat(targetPath); err == nil {
				return fmt.Errorf("conversion target already exists: %s", targetPath)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		files = append(files, conversionFile{source: path, target: targetPath, data: encoded, orig: original, mode: info.Mode().Perm()})
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, errors.New("no structured config files found to convert")
	}

	written := make([]conversionFile, 0, len(files))
	rollbackTargets := func() {
		for _, file := range written {
			if file.target != file.source {
				_ = os.Remove(file.target)
			}
		}
	}
	for _, file := range files {
		if err := state.WriteFileAtomic(file.target, file.data, file.mode); err != nil {
			rollbackTargets()
			return 0, fmt.Errorf("write converted file %s: %w", file.target, err)
		}
		written = append(written, file)
	}

	removed := make([]conversionFile, 0, len(files))
	for _, file := range files {
		if file.source == file.target {
			continue
		}
		if err := os.Remove(file.source); err != nil {
			for _, restore := range removed {
				_ = state.WriteFileAtomic(restore.source, restore.orig, restore.mode)
			}
			rollbackTargets()
			return 0, fmt.Errorf("remove old structured file %s: %w", file.source, err)
		}
		removed = append(removed, file)
	}
	return len(files), nil
}
