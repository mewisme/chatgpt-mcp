package licenseinventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

const apache20SHA256 = "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30"

func TestRootLicenseIsCanonicalApache20(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != apache20SHA256 {
		t.Fatalf("LICENSE sha256 = %s, want canonical Apache-2.0", hex.EncodeToString(sum[:]))
	}
}

func TestOfficialPluginLicensesMatchPolicy(t *testing.T) {
	root := moduleRoot(t)
	want := map[string]string{
		"admin-ui": "Apache-2.0", "bash": "GPL-2.0-only", "rtk": "Apache-2.0", "cf-tunnel": "Apache-2.0",
		"secure-mcp-tunnel": "Apache-2.0", "tui": "Apache-2.0", "markdown-formatter": "Apache-2.0",
		"ponytail": "Apache-2.0", "caveman": "Apache-2.0",
	}
	for id, license := range want {
		manifest, err := plugin.ParseManifest(mustRead(t, filepath.Join(root, "plugins", id, "plugin.json")))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if manifest.License != license {
			t.Fatalf("%s license = %q, want %q", id, manifest.License, license)
		}
	}
}

func TestCopiedSourceTreesKeepUpstreamLicenseFiles(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range []string{
		"plugins/cf-tunnel/internal/cloudflared/LICENSE",
		"plugins/cf-tunnel/internal/cloudflared/UPSTREAM.md",
		"third_party/ponytail/LICENSE",
		"third_party/ponytail/NOTICE.md",
		"third_party/caveman/LICENSE",
		"third_party/caveman/NOTICE.md",
		"plugins/bash/licenses/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoReleaserShipsGeneratedCoreComplianceFiles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, needle := range []string{
		"go run ./internal/licenseinventory/cmd/license-inventory --artifact core",
		"dist/licenses/core/NOTICE",
		"dist/licenses/core/licenses.txt",
		"dist/licenses/core/sbom.spdx.json",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("goreleaser missing %q", needle)
		}
	}
	if strings.Contains(text, "third_party/ponytail") || strings.Contains(text, "third_party/caveman") {
		t.Fatal("core archive still vendors plugin third_party notices")
	}
}

func TestWrittenSBOMIsSPDXJSON(t *testing.T) {
	inv := Inventory{Artifact: "core", Packages: []Package{
		{Name: "core", Version: "dev", License: "Apache-2.0", Kind: KindRoot},
		{Name: "example.com/mod", Version: "v1.0.0", License: "MIT", Kind: KindGoModule},
	}}
	out := t.TempDir()
	if err := Write(inv, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "sbom.spdx.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SPDXVersion string `json:"spdxVersion"`
		Packages    []struct {
			Name             string `json:"name"`
			LicenseConcluded string `json:"licenseConcluded"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SPDXVersion != "SPDX-2.3" || len(doc.Packages) != 2 || doc.Packages[1].LicenseConcluded != "MIT" {
		t.Fatalf("sbom = %#v", doc)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
