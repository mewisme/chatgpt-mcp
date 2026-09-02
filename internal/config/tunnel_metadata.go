package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TunnelMetadataDir() string { return filepath.Join(RootPath(), "tunnels") }

func TunnelMetadataPath(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("tunnel id is required")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\\`) || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid tunnel id %q", id)
	}
	source, err := Source()
	if err != nil {
		return "", err
	}
	ext := source.Ext
	if ext == "" {
		ext = configformat.Extension(source.Format)
	}
	if ext == "" {
		ext = ".json"
	}
	return filepath.Join(TunnelMetadataDir(), id+ext), nil
}

func LoadTunnelMetadata(id string) (tunnel.Metadata, error) {
	path, err := TunnelMetadataPath(id)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	var metadata tunnel.Metadata
	if err := configformat.UnmarshalPath(path, data, &metadata); err != nil {
		return tunnel.Metadata{}, fmt.Errorf("decode tunnel metadata %s: %w", path, err)
	}
	if strings.TrimSpace(metadata.ID) == "" {
		metadata.ID = strings.TrimSpace(id)
	}
	if metadata.ID != strings.TrimSpace(id) {
		return tunnel.Metadata{}, fmt.Errorf("tunnel metadata id mismatch: file %s contains %s", id, metadata.ID)
	}
	return metadata, nil
}

func SaveTunnelMetadata(metadata tunnel.Metadata) (string, error) {
	metadata.ID = strings.TrimSpace(metadata.ID)
	path, err := TunnelMetadataPath(metadata.ID)
	if err != nil {
		return "", err
	}
	if metadata.FetchedAt.IsZero() {
		metadata.FetchedAt = time.Now().UTC()
	}
	data, err := configformat.MarshalPath(path, metadata)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := state.WriteFileAtomic(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

func RemoveTunnelMetadata(id string) error {
	path, err := TunnelMetadataPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func SyncTunnelMetadata(ctx context.Context, cfg tunnel.Config) (tunnel.Metadata, string, error) {
	if !tunnel.Configured(cfg) {
		return tunnel.Metadata{}, "", errors.New("tunnel id and runtime API key are required to sync metadata")
	}
	metadata, err := tunnel.FetchMetadata(ctx, cfg)
	if err != nil {
		return tunnel.Metadata{}, "", err
	}
	path, err := SaveTunnelMetadata(metadata)
	if err != nil {
		return tunnel.Metadata{}, "", err
	}
	return metadata, path, nil
}
