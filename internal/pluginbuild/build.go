package pluginbuild

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/licenseinventory"
	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type Request struct {
	RepoRoot     string
	TemplatePath string
	OutputRoot   string
	OnlyPlatform string
	Package      string
}

func Build(req Request) (string, error) {
	data, err := os.ReadFile(req.TemplatePath)
	if err != nil {
		return "", fmt.Errorf("read plugin template: %w", err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("decode plugin template: %w", err)
	}
	if err := os.MkdirAll(req.OutputRoot, 0755); err != nil {
		return "", err
	}
	extras, err := licenseinventory.ExtraFiles(req.RepoRoot, string(manifest.ID))
	if err != nil {
		return "", err
	}
	platforms := make([]string, 0, len(manifest.Platforms))
	for name := range manifest.Platforms {
		platforms = append(platforms, name)
	}
	sort.Strings(platforms)
	for _, name := range platforms {
		if req.OnlyPlatform != "" && name != req.OnlyPlatform {
			continue
		}
		artifact := manifest.Platforms[name]
		if artifact.SHA256 != DigestSentinel {
			return "", fmt.Errorf("plugin template %s sha256 must use the generated digest sentinel", name)
		}
		goos, goarch, ok := strings.Cut(name, "/")
		if !ok {
			return "", fmt.Errorf("invalid platform %s", name)
		}
		staging := filepath.Join(req.OutputRoot, ".staging-"+strings.ReplaceAll(name, "/", "-"))
		if err := os.RemoveAll(staging); err != nil {
			return "", err
		}
		if err := os.MkdirAll(staging, 0755); err != nil {
			return "", err
		}
		binary := filepath.Join(staging, filepath.FromSlash(artifact.Entrypoint))
		if err := compile(req.RepoRoot, req.Package, goos, goarch, binary); err != nil {
			return "", err
		}
		if err := compressBinary(binary, goos); err != nil {
			return "", err
		}
		artifactPath := filepath.Join(req.OutputRoot, artifact.Artifact)
		if err := zipFile(binary, artifact.Entrypoint, artifactPath, extras); err != nil {
			return "", err
		}
		digest, err := fileSHA256(artifactPath)
		if err != nil {
			return "", err
		}
		artifact.SHA256 = digest
		manifest.Platforms[name] = artifact
		_ = os.RemoveAll(staging)
	}
	if req.OnlyPlatform != "" {
		if _, ok := manifest.Platforms[req.OnlyPlatform]; !ok {
			return "", fmt.Errorf("unknown platform %s", req.OnlyPlatform)
		}
	}
	if err := manifest.Validate(); err != nil {
		return "", fmt.Errorf("validate generated plugin manifest: %w", err)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	manifestPath := filepath.Join(req.OutputRoot, fmt.Sprintf("%s-%s.json", manifest.ID, manifest.Version))
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0644); err != nil {
		return "", err
	}
	return manifestPath, nil
}

func compile(repoRoot, pkg, goos, goarch, output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", output, pkg)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=-buildvcs=false")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build %s/%s: %w", goos, goarch, err)
	}
	return nil
}

func zipFile(sourcePath, entryName, outputPath string, extras []licenseinventory.ExtraFile) error {
	temp, err := os.CreateTemp(filepath.Dir(outputPath), ".plugin-*.zip")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	writer := zip.NewWriter(temp)
	info, err := os.Stat(sourcePath)
	if err != nil {
		_ = writer.Close()
		_ = temp.Close()
		return err
	}
	header := &zip.FileHeader{Name: entryName, Method: zip.Deflate, Modified: zipEpoch}
	header.SetMode(info.Mode() | 0755)
	destination, err := writer.CreateHeader(header)
	if err != nil {
		_ = writer.Close()
		_ = temp.Close()
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		_ = writer.Close()
		_ = temp.Close()
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := source.Close()
	if copyErr != nil {
		_ = writer.Close()
		_ = temp.Close()
		return copyErr
	}
	if closeErr != nil {
		_ = writer.Close()
		_ = temp.Close()
		return closeErr
	}
	if err := licenseinventory.AddZipFiles(writer, extras, zipEpoch); err != nil {
		_ = writer.Close()
		_ = temp.Close()
		return err
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
