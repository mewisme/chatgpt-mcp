package plugindev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type fingerprintInput struct {
	Platform  string   `json:"platform"`
	GoVersion string   `json:"go_version,omitempty"`
	Node      string   `json:"node_version,omitempty"`
	Files     []string `json:"files"`
}

func Fingerprint(repoRoot, pluginID, platform string) (string, error) {
	root, err := verifiedRoot(repoRoot)
	if err != nil {
		return "", err
	}
	pluginDir := filepath.Join(root, "plugins", pluginID)
	info, err := os.Stat(pluginDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("plugin %s source is not inside the repository", pluginID)
	}
	files := map[string]string{}
	if err := hashTree(root, pluginDir, files); err != nil {
		return "", err
	}
	for _, name := range []string{"go.mod", "go.sum", "plugins/workflow.json", "plugins/" + pluginID + "/plugin.json"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if _, err := os.Stat(path); err == nil {
			if err := hashFile(root, path, files); err != nil {
				return "", err
			}
		}
	}
	goVersion := ""
	if usesGo(pluginDir) {
		goVersion, err = toolchainVersion(root, "go", "version")
		if err != nil {
			return "", err
		}
		if err := hashGoDeps(root, pluginID, files); err != nil {
			return "", err
		}
	}
	nodeVersion := ""
	if usesNode(pluginDir) {
		nodeVersion, err = toolchainVersion(root, "node", "-v")
		if err != nil {
			return "", err
		}
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]string, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, key+"="+files[key])
	}
	payload, err := json.Marshal(fingerprintInput{Platform: platform, GoVersion: goVersion, Node: nodeVersion, Files: ordered})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func CacheDir(repoRoot, pluginID, platform, fingerprint string) (string, error) {
	root, err := verifiedRoot(repoRoot)
	if err != nil {
		return "", err
	}
	if fingerprint == "" || strings.Contains(fingerprint, string(filepath.Separator)) || strings.Contains(fingerprint, "..") {
		return "", fmt.Errorf("invalid fingerprint")
	}
	return filepath.Join(root, ".cgm", "dev", "builds", strings.ReplaceAll(platform, "/", "-"), pluginID, fingerprint), nil
}

func usesGo(pluginDir string) bool {
	matches, _ := filepath.Glob(filepath.Join(pluginDir, "*.go"))
	if len(matches) > 0 {
		return true
	}
	_, err := os.Stat(filepath.Join(pluginDir, "cmd"))
	return err == nil
}

func usesNode(pluginDir string) bool {
	_, err := os.Stat(filepath.Join(pluginDir, "package.json"))
	return err == nil
}

func toolchainVersion(root, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CHATGPT_MCP_DEV_PLUGINS=off")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", command, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func hashGoDeps(root, pluginID string, files map[string]string) error {
	cmd := exec.Command("go", "list", "-e", "-deps", "-f", "{{.Dir}}", "./plugins/"+pluginID+"/...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CHATGPT_MCP_DEV_PLUGINS=off")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("go list deps %s: %w", pluginID, err)
	}
	seen := map[string]bool{}
	for _, dir := range strings.Split(string(out), "\n") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		if err := hashTree(root, dir, files); err != nil {
			return err
		}
	}
	return nil
}

func hashTree(root, dir string, files map[string]string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "node_modules" || name == "dist" || name == ".cgm" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		return hashFile(root, path, files)
	})
}

func hashFile(root, path string, files map[string]string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if files[rel] != "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	files[rel] = hex.EncodeToString(sum.Sum(nil))
	return nil
}
