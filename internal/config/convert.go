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

type structuredFile struct {
	path   string
	base   string
	format configformat.Format
	ext    string
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
	structured, err := collectStructuredFiles(root)
	if err != nil {
		return 0, err
	}
	files := make([]conversionFile, 0, len(structured))
	targets := map[string]string{}
	for _, item := range structured {
		targetPath := filepath.Join(filepath.Dir(item.path), item.base+targetExt)
		if previous, exists := targets[targetPath]; exists && previous != item.path {
			return 0, fmt.Errorf("multiple structured files map to conversion target %s: %s, %s", targetPath, previous, item.path)
		}
		targets[targetPath] = item.path
		original, err := os.ReadFile(item.path)
		if err != nil {
			return 0, err
		}
		raw, err := configformat.DecodeGeneric(item.format, original)
		if err != nil {
			return 0, fmt.Errorf("decode %s: %w", item.path, err)
		}
		if item.base == "upstream" {
			if values, ok := raw.([]any); ok {
				raw = map[string]any{"servers": values}
			}
		}
		if targetPath == item.path {
			continue
		}
		if _, err := os.Stat(targetPath); err == nil {
			return 0, fmt.Errorf("conversion target already exists: %s", targetPath)
		} else if !os.IsNotExist(err) {
			return 0, err
		}
		encoded, err := configformat.EncodeGeneric(target, raw)
		if err != nil {
			return 0, fmt.Errorf("encode %s as %s: %w", item.path, target, err)
		}
		info, err := os.Stat(item.path)
		if err != nil {
			return 0, err
		}
		files = append(files, conversionFile{source: item.path, target: targetPath, data: encoded, orig: original, mode: info.Mode().Perm()})
	}
	if len(files) == 0 {
		return 0, nil
	}

	written := make([]conversionFile, 0, len(files))
	rollbackTargets := func() {
		for _, file := range written {
			_ = os.Remove(file.target)
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

func collectStructuredFiles(root string) ([]structuredFile, error) {
	files := make([]structuredFile, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" && ext != ".toml" {
			return nil
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !structuredStateNames[base] && !isTunnelMetadataFile(root, path) {
			return nil
		}
		format, err := configformat.Detect(path)
		if err != nil {
			return err
		}
		files = append(files, structuredFile{path: path, base: base, format: format, ext: filepath.Ext(entry.Name())})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no structured config files found")
	}
	return files, nil
}

func isTunnelMetadataFile(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return filepath.Dir(relative) == "tunnels"
}
