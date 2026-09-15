package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/validate-plugin-registry.go <index.json> <publishers.json> <assets-dir>")
		os.Exit(2)
	}
	if err := validatePluginRegistry(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fail(err)
	}
}

func validatePluginRegistry(indexPath, publishersPath, assetsRoot string) error {
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	publishersData, err := os.ReadFile(publishersPath)
	if err != nil {
		return err
	}
	index, err := plugin.ParseRegistryIndex(indexData)
	if err != nil {
		return err
	}
	publishers, err := plugin.ParsePublisherIndex(publishersData)
	if err != nil {
		return err
	}
	snapshot := plugin.RegistrySnapshot{Registry: plugin.OfficialRegistry(), Index: index, Publishers: publishers}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	return validateRegistryAssets(index, assetsRoot)
}

func validateRegistryAssets(index plugin.RegistryIndex, assetsRoot string) error {
	assetsRoot = strings.TrimSpace(assetsRoot)
	if assetsRoot == "" {
		return fmt.Errorf("plugin assets directory is required")
	}
	assetsRoot = filepath.Clean(assetsRoot)
	referenced := map[string]struct{}{}
	ids := make([]string, 0, len(index.Plugins))
	for id := range index.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, idValue := range ids {
		id := plugin.PluginID(idValue)
		entry := index.Plugins[id]
		versions := make([]string, 0, len(entry.Versions))
		for version := range entry.Versions {
			versions = append(versions, string(version))
		}
		sort.Strings(versions)
		for _, versionValue := range versions {
			version := plugin.Version(versionValue)
			manifestName := entry.Versions[version]
			referenced[manifestName] = struct{}{}
			manifestPath := filepath.Join(assetsRoot, manifestName)
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				if version == entry.Stable || version == entry.Beta {
					return fmt.Errorf("registry channel manifest %s/%s is unavailable: %w", id, version, err)
				}
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			manifest, err := plugin.ParseManifest(data)
			if err != nil {
				return fmt.Errorf("validate manifest %s: %w", manifestName, err)
			}
			if manifest.ID != id || manifest.Version != version || manifest.Publisher != entry.Publisher || manifest.Type != entry.Type {
				return fmt.Errorf("registry manifest %s identity does not match index entry %s@%s", manifestName, id, version)
			}
			platforms := make([]string, 0, len(manifest.Platforms))
			for platform := range manifest.Platforms {
				platforms = append(platforms, platform)
			}
			sort.Strings(platforms)
			for _, platform := range platforms {
				artifact := manifest.Platforms[platform]
				if artifact.HostBacked() {
					continue
				}
				artifactPath := filepath.Join(assetsRoot, artifact.Artifact)
				digest, err := fileSHA256(artifactPath)
				if err != nil {
					return fmt.Errorf("registry artifact %s for %s@%s %s is unavailable: %w", artifact.Artifact, id, version, platform, err)
				}
				if !strings.EqualFold(digest, artifact.SHA256) {
					return fmt.Errorf("registry artifact %s digest mismatch: manifest=%s actual=%s", artifact.Artifact, artifact.SHA256, digest)
				}
			}
		}
	}
	manifests, err := filepath.Glob(filepath.Join(assetsRoot, "*.json"))
	if err != nil {
		return err
	}
	for _, manifestPath := range manifests {
		name := filepath.Base(manifestPath)
		if name == "index.json" || name == "publishers.json" || strings.HasSuffix(name, ".sigstore.json") {
			continue
		}
		if _, ok := referenced[name]; !ok {
			return fmt.Errorf("plugin manifest asset %s is not referenced by index.json", name)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
