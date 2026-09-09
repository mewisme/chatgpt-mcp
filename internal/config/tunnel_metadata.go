package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
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
	return SaveTunnelMetadataContext(context.Background(), metadata)
}

func SaveTunnelMetadataContext(ctx context.Context, metadata tunnel.Metadata) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	metadata.ID = strings.TrimSpace(metadata.ID)
	path, err := TunnelMetadataPath(metadata.ID)
	if err != nil {
		return "", err
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.metadata.persist", "Persisting tunnel metadata cache", tracepkg.String("tunnel_id", metadata.ID), tracepkg.String("path", path), tracepkg.Bool("atomic", true))
	if metadata.FetchedAt.IsZero() {
		metadata.FetchedAt = time.Now().UTC()
	}
	data, err := configformat.MarshalPath(path, metadata)
	if err != nil {
		span.FailMessage("Tunnel metadata cache encoding failed", err)
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		span.FailMessage("Tunnel metadata cache directory creation failed", err)
		return "", err
	}
	if err := state.WriteFileAtomic(path, data, 0600); err != nil {
		span.FailMessage("Tunnel metadata cache persistence failed", err, tracepkg.Int64("bytes", int64(len(data))))
		return "", err
	}
	format := ""
	if detected, detectErr := configformat.Detect(path); detectErr == nil {
		format = string(detected)
	}
	span.EndMessage("Tunnel metadata cache persisted", tracepkg.String("format", format), tracepkg.Int64("bytes", int64(len(data))))
	return path, nil
}

func RemoveTunnelMetadata(id string) error {
	return RemoveTunnelMetadataContext(context.Background(), id)
}

func RemoveTunnelMetadataContext(ctx context.Context, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := TunnelMetadataPath(id)
	if err != nil {
		return err
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.metadata.remove", "Removing tunnel metadata cache", tracepkg.String("tunnel_id", strings.TrimSpace(id)), tracepkg.String("path", path))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		span.FailMessage("Tunnel metadata cache removal failed", err)
		return err
	}
	span.EndMessage("Tunnel metadata cache removed")
	return nil
}

func ListTunnelMetadata() ([]tunnel.Metadata, error) {
	dir := TunnelMetadataDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []tunnel.Metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := make([]tunnel.Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if _, err := configformat.Detect(path); err != nil {
			continue
		}
		file, err := root.Open(entry.Name())
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		var metadata tunnel.Metadata
		if err := configformat.UnmarshalPath(path, data, &metadata); err != nil {
			return nil, fmt.Errorf("decode tunnel metadata %s: %w", path, err)
		}
		if strings.TrimSpace(metadata.ID) == "" {
			metadata.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if _, err := TunnelMetadataPath(metadata.ID); err != nil {
			return nil, err
		}
		result = append(result, metadata)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func SyncTunnelMetadata(ctx context.Context, cfg tunnel.Config) (tunnel.Metadata, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.metadata.sync", "Synchronizing configured tunnel metadata", tracepkg.String("tunnel_id", strings.TrimSpace(cfg.ID)), tracepkg.URL("control_plane_base_url", cfg.ControlPlaneBaseURL))
	if !tunnel.Configured(cfg) {
		err := errors.New("tunnel id and runtime API key are required to sync metadata")
		span.FailMessage("Tunnel metadata synchronization validation failed", err)
		return tunnel.Metadata{}, "", err
	}
	metadata, err := tunnel.FetchMetadata(ctx, cfg)
	if err != nil {
		span.FailMessage("Tunnel metadata fetch failed", err)
		return tunnel.Metadata{}, "", err
	}
	path, err := SaveTunnelMetadataContext(ctx, metadata)
	if err != nil {
		span.FailMessage("Tunnel metadata cache persistence failed", err)
		return tunnel.Metadata{}, "", err
	}
	span.EndMessage("Configured tunnel metadata synchronized", tracepkg.String("tunnel_id", metadata.ID), tracepkg.String("metadata_path", path))
	return metadata, path, nil
}
