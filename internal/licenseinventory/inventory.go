package licenseinventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	KindGoModule = "go-module"
	KindNPM      = "npm"
	KindCopied   = "copied"
	KindRoot     = "root"
)

type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
	Path    string `json:"path"`
	Source  string `json:"source,omitempty"`
	Kind    string `json:"kind"`
	Text    string `json:"-"`
}

type Inventory struct {
	Artifact string    `json:"artifact"`
	Packages []Package `json:"packages"`
	Notices  []string  `json:"-"`
}

type AllowedLicense struct {
	SPDX      string   `json:"spdx"`
	Kind      string   `json:"kind"`
	Rationale string   `json:"rationale,omitempty"`
	Artifacts []string `json:"artifacts,omitempty"`
}

type Policy struct {
	Allowed []AllowedLicense `json:"allowed"`
}

type CopiedSource struct {
	Name       string
	Version    string
	License    string
	Path       string
	Source     string
	NoticePath string
}

type Artifact struct {
	ID          string
	GoPackages  []string
	NPMRoot     string
	Copied      []CopiedSource
	RootLicense string
	RootName    string
	RootVersion string
}

type Collector struct {
	Root    string
	GoList  func(root string, packages []string) ([]Package, error)
	NPMList func(root, npmRoot string) ([]Package, error)
}

func Artifacts() []Artifact {
	return []Artifact{
		{ID: "core", GoPackages: []string{"."}, RootLicense: "Apache-2.0", RootName: "go.mewis.me/chatgpt-mcp", RootVersion: "core"},
		{ID: "admin-ui", NPMRoot: "plugins/admin-ui", RootLicense: "Apache-2.0", RootName: "admin-ui", RootVersion: "plugin"},
		{ID: "secure-mcp-tunnel", GoPackages: []string{"./plugins/secure-mcp-tunnel/cmd/secure-mcp-tunnel"}, RootLicense: "Apache-2.0", RootName: "secure-mcp-tunnel", RootVersion: "plugin"},
		{ID: "tui", GoPackages: []string{"./plugins/tui/cmd/tui"}, RootLicense: "Apache-2.0", RootName: "tui", RootVersion: "plugin"},
		{ID: "markdown-formatter", GoPackages: []string{"./plugins/markdown-formatter/cmd/markdown-formatter"}, RootLicense: "Apache-2.0", RootName: "markdown-formatter", RootVersion: "plugin"},
		{ID: "ponytail", GoPackages: []string{"./plugins/ponytail/cmd/ponytail"}, RootLicense: "Apache-2.0", RootName: "ponytail", RootVersion: "plugin", Copied: []CopiedSource{{
			Name: "third_party/ponytail", Version: "974d940", License: "MIT", Path: "third_party/ponytail", Source: "https://github.com/DietrichGebert/ponytail", NoticePath: "third_party/ponytail/NOTICE.md",
		}}},
		{ID: "caveman", GoPackages: []string{"./plugins/caveman/cmd/caveman"}, RootLicense: "Apache-2.0", RootName: "caveman", RootVersion: "plugin", Copied: []CopiedSource{{
			Name: "third_party/caveman", Version: "5184b3d", License: "MIT", Path: "third_party/caveman", Source: "https://github.com/JuliusBrussee/caveman", NoticePath: "third_party/caveman/NOTICE.md",
		}}},
		{ID: "cf-tunnel", GoPackages: []string{"./plugins/cf-tunnel/cmd/cf-tunnel"}, RootLicense: "Apache-2.0", RootName: "cf-tunnel", RootVersion: "plugin", Copied: []CopiedSource{{
			Name: "plugins/cf-tunnel/internal/cloudflared", Version: "2026.9.1", License: "Apache-2.0", Path: "plugins/cf-tunnel/internal/cloudflared", Source: "https://github.com/cloudflare/cloudflared",
		}}},
		{ID: "bash", RootLicense: "GPL-2.0-only", RootName: "bash", RootVersion: "plugin", Copied: []CopiedSource{{
			Name: "plugins/bash", Version: "portablegit", License: "GPL-2.0-only", Path: "plugins/bash/licenses", Source: "https://gitforwindows.org/faq.html#licenses",
		}}},
		{ID: "rtk", RootLicense: "Apache-2.0", RootName: "rtk", RootVersion: "plugin"},
	}
}

func ArtifactByID(id string) (Artifact, bool) {
	for _, artifact := range Artifacts() {
		if artifact.ID == id {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func LoadPolicy(root string) (Policy, error) {
	data, err := os.ReadFile(filepath.Join(root, "licenses", "policy.json"))
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	if len(policy.Allowed) == 0 {
		return Policy{}, errors.New("license policy allowlist is empty")
	}
	return policy, nil
}

func (collector Collector) Generate(artifactID string) (Inventory, error) {
	artifact, ok := ArtifactByID(artifactID)
	if !ok {
		return Inventory{}, fmt.Errorf("unknown license inventory artifact %q", artifactID)
	}
	goList := collector.GoList
	if goList == nil {
		goList = listGoModules
	}
	npmList := collector.NPMList
	if npmList == nil {
		npmList = listNPMPackages
	}
	var packages []Package
	var notices []string
	packages = append(packages, Package{Name: artifact.RootName, Version: artifact.RootVersion, License: artifact.RootLicense, Path: artifact.ID, Kind: KindRoot})
	if len(artifact.GoPackages) > 0 {
		mods, err := goList(collector.Root, artifact.GoPackages)
		if err != nil {
			return Inventory{}, err
		}
		packages = append(packages, mods...)
	}
	if artifact.NPMRoot != "" {
		mods, err := npmList(collector.Root, artifact.NPMRoot)
		if err != nil {
			return Inventory{}, err
		}
		packages = append(packages, mods...)
	}
	for _, copied := range artifact.Copied {
		pkg := Package{Name: copied.Name, Version: copied.Version, License: copied.License, Path: copied.Path, Source: copied.Source, Kind: KindCopied}
		if text, err := copiedLicenseText(collector.Root, copied.Path); err != nil {
			return Inventory{}, err
		} else {
			pkg.Text = text
		}
		if copied.NoticePath != "" {
			notice, err := os.ReadFile(filepath.Join(collector.Root, copied.NoticePath))
			if err != nil {
				return Inventory{}, err
			}
			notices = append(notices, strings.TrimSpace(string(notice)))
		}
		packages = append(packages, pkg)
	}
	packages = dedupePackages(packages)
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name != packages[j].Name {
			return packages[i].Name < packages[j].Name
		}
		if packages[i].Version != packages[j].Version {
			return packages[i].Version < packages[j].Version
		}
		return packages[i].Kind < packages[j].Kind
	})
	return Inventory{Artifact: artifactID, Packages: packages, Notices: notices}, nil
}

func Validate(inv Inventory, policy Policy) error {
	allowed := map[string]AllowedLicense{}
	for _, entry := range policy.Allowed {
		if strings.TrimSpace(entry.SPDX) == "" {
			return errors.New("license policy entry is missing spdx")
		}
		if strings.TrimSpace(entry.Kind) == "" {
			return fmt.Errorf("license policy %s is missing kind", entry.SPDX)
		}
		if (entry.Kind == "copyleft" || entry.Kind == "custom" || strings.Contains(strings.ToUpper(entry.SPDX), "GPL") || strings.Contains(strings.ToUpper(entry.SPDX), "AGPL") || strings.Contains(strings.ToUpper(entry.SPDX), "SSPL") || strings.Contains(strings.ToUpper(entry.SPDX), "BUSL")) && strings.TrimSpace(entry.Rationale) == "" {
			return fmt.Errorf("restrictive license %s requires rationale", entry.SPDX)
		}
		allowed[entry.SPDX] = entry
	}
	for _, pkg := range inv.Packages {
		if strings.TrimSpace(pkg.License) == "" {
			return fmt.Errorf("%s@%s: unclassified license", pkg.Name, pkg.Version)
		}
		for _, token := range licenseTokens(pkg.License) {
			entry, ok := allowed[token]
			if !ok {
				return fmt.Errorf("%s@%s: license %s is not allowlisted", pkg.Name, pkg.Version, token)
			}
			if len(entry.Artifacts) > 0 && !contains(entry.Artifacts, inv.Artifact) {
				return fmt.Errorf("%s@%s: license %s is not allowed on artifact %s", pkg.Name, pkg.Version, token, inv.Artifact)
			}
		}
	}
	return nil
}

func Write(inv Inventory, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "license-inventory.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "NOTICE"), []byte(renderNOTICE(inv)), 0644); err != nil {
		return err
	}
	sbom, err := json.MarshalIndent(renderSBOM(inv), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "sbom.spdx.json"), append(sbom, '\n'), 0644); err != nil {
		return err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# chatgpt-mcp %s third-party licenses\n", inv.Artifact)
	for _, pkg := range inv.Packages {
		fmt.Fprintf(&builder, "\n================================================================================\n%s %s\nSPDX: %s\nPath: %s\n", pkg.Name, pkg.Version, pkg.License, pkg.Path)
		if pkg.Source != "" {
			fmt.Fprintf(&builder, "Source: %s\n", pkg.Source)
		}
		builder.WriteString("================================================================================\n")
		text := strings.TrimSpace(pkg.Text)
		if text == "" {
			text = pkg.License
		}
		builder.WriteString(text)
		builder.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(outputDir, "licenses.txt"), []byte(builder.String()), 0644)
}

type goListPackage struct {
	ImportPath string
	Standard   bool
	Module     *goListModule
}

type goListModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
}

func listGoModules(root string, packages []string) ([]Package, error) {
	args := append([]string{"list", "-deps", "-e", "-json=ImportPath,Standard,Module"}, packages...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOTELEMETRY=off")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("go list: %w\n%s", err, exit.Stderr)
		}
		return nil, fmt.Errorf("go list: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(out))
	seen := map[string]struct{}{}
	var result []Package
	for {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list: %w", err)
		}
		if pkg.Standard || pkg.Module == nil || pkg.Module.Main || pkg.Module.Path == "" {
			continue
		}
		key := pkg.Module.Path + "@" + pkg.Module.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		licenseName, text, err := readLicenseFile(pkg.Module.Dir)
		if err != nil {
			return nil, err
		}
		item := Package{Name: pkg.Module.Path, Version: pkg.Module.Version, Path: pkg.Module.Path, Source: "https://" + pkg.Module.Path, Kind: KindGoModule, Text: text}
		item.License = classifyLicense(text)
		if item.License == "" && licenseName != "" {
			item.License = classifyLicense(licenseName)
		}
		result = append(result, item)
	}
	return result, nil
}

type npmListNode struct {
	Name         string                  `json:"name"`
	Version      string                  `json:"version"`
	Path         string                  `json:"path"`
	Dependencies map[string]*npmListNode `json:"dependencies"`
}

func listNPMPackages(root, npmRoot string) ([]Package, error) {
	dir := filepath.Join(root, npmRoot)
	cmd := exec.Command("pnpm", "list", "--prod", "--json", "--depth", "Infinity")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("pnpm list: %w\n%s", err, exit.Stderr)
		}
		return nil, fmt.Errorf("pnpm list: %w", err)
	}
	var tree npmListNode
	if err := json.Unmarshal(out, &tree); err != nil {
		var trees []npmListNode
		if err := json.Unmarshal(out, &trees); err != nil {
			return nil, fmt.Errorf("decode pnpm list: %w", err)
		}
		if len(trees) == 0 {
			return nil, errors.New("pnpm list returned no packages")
		}
		tree = trees[0]
	}
	seen := map[string]struct{}{}
	var result []Package
	var walkErr error
	var walk func(node *npmListNode)
	walk = func(node *npmListNode) {
		if node == nil || walkErr != nil {
			return
		}
		for name, child := range node.Dependencies {
			if child == nil {
				continue
			}
			if child.Name == "" {
				child.Name = name
			}
			key := child.Name + "@" + child.Version
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				pkgDir := strings.TrimSpace(child.Path)
				if pkgDir == "" {
					walk(child)
					continue
				}
				manifest, err := readFileIfExists(filepath.Join(pkgDir, "package.json"))
				if err != nil {
					walkErr = err
					return
				}
				if manifest == "" || npmNativeAddon(manifest) {
					walk(child)
					continue
				}
				item := Package{Name: child.Name, Version: child.Version, Path: child.Name, Kind: KindNPM}
				item.License = npmPackageLicense(manifest)
				if _, text, err := readLicenseFile(pkgDir); err != nil {
					walkErr = err
					return
				} else {
					item.Text = text
					if item.License == "" || strings.HasPrefix(strings.ToUpper(item.License), "SEE LICENSE") {
						item.License = classifyLicense(text)
					}
				}
				if homepage := npmPackageHomepage(pkgDir); homepage != "" {
					item.Source = homepage
				}
				result = append(result, item)
			}
			walk(child)
		}
	}
	walk(&tree)
	if walkErr != nil {
		return nil, walkErr
	}
	return result, nil
}

func npmPackageLicense(data string) string {
	var parsed struct {
		License  any `json:"license"`
		Licenses any `json:"licenses"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return ""
	}
	if value := npmLicenseValue(parsed.License); value != "" {
		return value
	}
	return npmLicenseValue(parsed.Licenses)
}

func npmLicenseValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.Trim(strings.TrimSpace(typed), "()")
	case map[string]any:
		if name, ok := typed["type"].(string); ok {
			return strings.TrimSpace(name)
		}
	case []any:
		var parts []string
		for _, item := range typed {
			if part := npmLicenseValue(item); part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, " OR ")
	}
	return ""
}

func npmNativeAddon(data string) bool {
	var parsed struct {
		OS   []string `json:"os"`
		CPU  []string `json:"cpu"`
		Libc []string `json:"libc"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return false
	}
	return len(parsed.OS) > 0 || len(parsed.CPU) > 0 || len(parsed.Libc) > 0
}

func npmPackageHomepage(dir string) string {
	data, err := readFileIfExists(filepath.Join(dir, "package.json"))
	if err != nil || data == "" {
		return ""
	}
	var parsed struct {
		Homepage   string `json:"homepage"`
		Repository any    `json:"repository"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return ""
	}
	if strings.TrimSpace(parsed.Homepage) != "" {
		return strings.TrimSpace(parsed.Homepage)
	}
	switch value := parsed.Repository.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		if url, ok := value["url"].(string); ok {
			return strings.TrimSpace(url)
		}
	}
	return ""
}

func copiedLicenseText(root, rel string) (string, error) {
	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	_, text, err := readLicenseFile(path)
	if err != nil {
		return "", err
	}
	if text != "" {
		return text, nil
	}
	readme := filepath.Join(path, "README.md")
	data, err := readFileIfExists(readme)
	if err != nil {
		return "", err
	}
	return data, nil
}

func dedupePackages(packages []Package) []Package {
	type key struct{ name, version, kind string }
	seen := map[key]Package{}
	order := []key{}
	for _, pkg := range packages {
		item := key{pkg.Name, pkg.Version, pkg.Kind}
		if existing, ok := seen[item]; ok {
			if existing.License == "" && pkg.License != "" {
				seen[item] = pkg
			}
			continue
		}
		seen[item] = pkg
		order = append(order, item)
	}
	result := make([]Package, 0, len(order))
	for _, item := range order {
		result = append(result, seen[item])
	}
	return result
}

func licenseTokens(value string) []string {
	value = strings.NewReplacer("(", " ", ")", " ").Replace(value)
	fields := strings.Fields(value)
	var tokens []string
	for i, field := range fields {
		if i%2 == 1 {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
