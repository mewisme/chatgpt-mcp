package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

const generatedDigestSentinel = "0000000000000000000000000000000000000000000000000000000000000000"

var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func main() {
	var sourceRoot, templatePath, outputRoot string
	flag.StringVar(&sourceRoot, "source-root", "", "prepared PortableGit root")
	flag.StringVar(&templatePath, "template", "plugins/bash/plugin.json", "plugin manifest template")
	flag.StringVar(&outputRoot, "output", "dist/plugins", "release output directory")
	flag.Parse()
	if strings.TrimSpace(sourceRoot) == "" {
		fail(errors.New("--source-root is required"))
	}
	artifactPath, manifestPath, err := build(sourceRoot, templatePath, outputRoot)
	if err != nil {
		fail(err)
	}
	fmt.Printf("artifact=%s\nmanifest=%s\n", artifactPath, manifestPath)
}

func build(sourceRoot, templatePath, outputRoot string) (string, string, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", "", fmt.Errorf("read plugin template: %w", err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", fmt.Errorf("decode plugin template: %w", err)
	}
	artifact, ok := manifest.Platforms["windows/amd64"]
	if !ok {
		return "", "", errors.New("plugin template is missing windows/amd64")
	}
	if artifact.SHA256 != generatedDigestSentinel {
		return "", "", errors.New("plugin template sha256 must use the generated digest sentinel")
	}
	entrypoint := filepath.Join(sourceRoot, filepath.FromSlash(artifact.Entrypoint))
	info, err := os.Stat(entrypoint)
	if err != nil {
		return "", "", fmt.Errorf("prepared Bash entrypoint is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", errors.New("prepared Bash entrypoint is not a regular file")
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		return "", "", err
	}
	artifactPath := filepath.Join(outputRoot, artifact.Artifact)
	if err := deterministicZip(sourceRoot, artifactPath); err != nil {
		return "", "", err
	}
	digest, err := fileSHA256(artifactPath)
	if err != nil {
		return "", "", err
	}
	artifact.SHA256 = digest
	manifest.Platforms["windows/amd64"] = artifact
	if err := manifest.Validate(); err != nil {
		return "", "", fmt.Errorf("validate generated plugin manifest: %w", err)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", err
	}
	manifestPath := filepath.Join(outputRoot, fmt.Sprintf("%s-%s.json", manifest.ID, manifest.Version))
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0644); err != nil {
		return "", "", err
	}
	return artifactPath, manifestPath, nil
}

func deterministicZip(sourceRoot, outputPath string) error {
	paths := []string{}
	err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceRoot {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if machineSpecificPortablePath(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !entry.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported PortableGit entry type: %s", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	temp, err := os.CreateTemp(filepath.Dir(outputPath), ".bash-plugin-*.zip")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	writer := zip.NewWriter(temp)
	for _, path := range paths {
		if err := addZipEntry(writer, sourceRoot, path); err != nil {
			_ = writer.Close()
			_ = temp.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	_ = os.Remove(outputPath)
	return os.Rename(tempPath, outputPath)
}

func machineSpecificPortablePath(path string) bool {
	switch filepath.ToSlash(path) {
	case "etc/hosts", "etc/networks", "etc/protocols", "etc/services":
		return true
	default:
		return false
	}
}

func addZipEntry(writer *zip.Writer, root, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	name := filepath.ToSlash(relative)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: zipEpoch}
	if info.IsDir() {
		header.Name += "/"
		header.Method = zip.Store
		header.SetMode(0755 | os.ModeDir)
		_, err = writer.CreateHeader(header)
		return err
	}
	header.SetMode(0644)
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := source.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
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
