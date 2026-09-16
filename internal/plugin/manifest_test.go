package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidationAndPlatformSelection(t *testing.T) {
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	artifact, err := manifest.Platform("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Entrypoint != "bin/bash" {
		t.Fatalf("entrypoint = %q", artifact.Entrypoint)
	}
	if _, err := manifest.Platform("windows", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestManifestRejectsUnsafeMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(manifest *Manifest) { manifest.Schema = 999 }},
		{"id traversal", func(manifest *Manifest) { manifest.ID = "../bash" }},
		{"version prefix", func(manifest *Manifest) { manifest.Version = "v1.0.0" }},
		{"version build metadata", func(manifest *Manifest) { manifest.Version = "1.0.0+build" }},
		{"unknown permission", func(manifest *Manifest) { manifest.Permissions = []Permission{"system/root"} }},
		{"unknown capability", func(manifest *Manifest) { manifest.Provides = []Capability{"unknown/bash"} }},
		{"artifact traversal", func(manifest *Manifest) {
			artifact := manifest.Platforms["linux/amd64"]
			artifact.Artifact = "../bash.tar.gz"
			manifest.Platforms["linux/amd64"] = artifact
		}},
		{"entrypoint traversal", func(manifest *Manifest) {
			artifact := manifest.Platforms["linux/amd64"]
			artifact.Entrypoint = "../bin/bash"
			manifest.Platforms["linux/amd64"] = artifact
		}},
		{"windows entrypoint", func(manifest *Manifest) {
			artifact := manifest.Platforms["linux/amd64"]
			artifact.Entrypoint = `C:\\bash.exe`
			manifest.Platforms["linux/amd64"] = artifact
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := testManifest("bash", "1.0.0", "shell/bash")
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestHostPortableIntegrityModesAreExclusive(t *testing.T) {
	pinned := HostPortableInstall{URL: "https://example.test/tool.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "tool"}
	if err := validateHostPortable(pinned); err != nil {
		t.Fatal(err)
	}
	checksum := HostPortableInstall{URL: "https://example.test/tool.tar.gz", ChecksumURL: "https://example.test/checksums.txt", ChecksumAsset: "tool.tar.gz", Archive: "tar.gz", Entrypoint: "tool"}
	if err := validateHostPortable(checksum); err != nil {
		t.Fatal(err)
	}
	mixed := pinned
	mixed.ChecksumURL = checksum.ChecksumURL
	mixed.ChecksumAsset = checksum.ChecksumAsset
	if err := validateHostPortable(mixed); err == nil {
		t.Fatal("portable install accepted mixed pinned and checksum integrity modes")
	}
	missing := pinned
	missing.SHA256 = ""
	if err := validateHostPortable(missing); err == nil {
		t.Fatal("portable install accepted without integrity metadata")
	}
}

func TestManifestPlatformFallsBackToAnyAny(t *testing.T) {
	manifest := Manifest{
		Schema: ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "web-ui",
		Provides: []Capability{CapabilityWebUIAdmin}, Permissions: []Permission{},
		Scopes: []PluginScope{ScopeGlobal},
		Platforms: map[string]PlatformArtifact{
			"any/any":     {Artifact: "admin-ui-1.0.0.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "index.html"},
			"linux/amd64": {Artifact: "admin-ui-linux.zip", SHA256: strings.Repeat("b", 64), Archive: "zip", Entrypoint: "index.html"},
		},
	}
	artifact, err := manifest.Platform("windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Artifact != "admin-ui-1.0.0.zip" {
		t.Fatalf("fallback artifact = %q", artifact.Artifact)
	}
	artifact, err = manifest.Platform("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Artifact != "admin-ui-linux.zip" {
		t.Fatalf("exact artifact = %q", artifact.Artifact)
	}
}

func TestManifestWebUIIsolation(t *testing.T) {
	base := Manifest{
		Schema: ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "web-ui",
		Provides: []Capability{CapabilityWebUIAdmin}, Permissions: []Permission{},
		Scopes:    []PluginScope{ScopeGlobal},
		Platforms: map[string]PlatformArtifact{"any/any": {Artifact: "admin-ui-1.0.0.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "index.html"}},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	withPermission := base
	withPermission.Permissions = []Permission{PermissionProcessExecute}
	if err := withPermission.Validate(); err == nil {
		t.Fatal("web-ui runtime permission accepted")
	}
	wrongType := base
	wrongType.Type = "runtime"
	if err := wrongType.Validate(); err == nil {
		t.Fatal("web-ui capability on non-web-ui plugin accepted")
	}
	nonHTML := base
	nonHTML.Platforms = map[string]PlatformArtifact{"any/any": {Artifact: "admin-ui-1.0.0.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "app.js"}}
	if err := nonHTML.Validate(); err == nil {
		t.Fatal("web-ui non-HTML entrypoint accepted")
	}
}

func TestManifestCoreCompatibility(t *testing.T) {
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	manifest.Requires.ChatGPTMCP = ">=0.2.24"
	for version, want := range map[string]bool{"0.2.23": false, "0.2.24": true, "0.3.0": true} {
		got, err := manifest.CompatibleWithCore(version)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("CompatibleWithCore(%q) = %v, want %v", version, got, want)
		}
	}
}

func TestOfficialPluginManifestsDeclareIntendedScopes(t *testing.T) {
	cases := []struct {
		path   string
		scopes []PluginScope
	}{
		{filepath.Join("..", "..", "plugins", "admin-ui", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "bash", "plugin.json"), []PluginScope{ScopeGlobal, ScopeWorkspace}},
		{filepath.Join("..", "..", "plugins", "rtk", "plugin.json"), []PluginScope{ScopeGlobal, ScopeWorkspace}},
		{filepath.Join("..", "..", "plugins", "cf-tunnel", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "secure-mcp-tunnel", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "tui", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "markdown-formatter", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "ponytail", "plugin.json"), []PluginScope{ScopeGlobal}},
		{filepath.Join("..", "..", "plugins", "caveman", "plugin.json"), []PluginScope{ScopeGlobal}},
	}
	for _, test := range cases {
		data, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := ParseManifest(data)
		if err != nil {
			t.Fatalf("%s: %v", test.path, err)
		}
		if manifest.Schema != ManifestSchema {
			t.Fatalf("%s schema = %d, want %d", test.path, manifest.Schema, ManifestSchema)
		}
		got := manifest.AllowedScopes()
		if len(got) != len(test.scopes) {
			t.Fatalf("%s scopes = %#v, want %#v", test.path, got, test.scopes)
		}
		for i := range test.scopes {
			if got[i] != test.scopes[i] {
				t.Fatalf("%s scopes = %#v, want %#v", test.path, got, test.scopes)
			}
		}
	}
}

func TestManifestSchema1RequiresValidScopes(t *testing.T) {
	valid := testManifest("bash", "1.0.0", "shell/bash")
	valid.Scopes = []PluginScope{ScopeGlobal, ScopeWorkspace}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if !valid.AllowsScope(ScopeWorkspace) {
		t.Fatal("schema 1 workspace scope rejected")
	}
	tests := []struct {
		name   string
		scopes []PluginScope
	}{
		{"empty", nil},
		{"unknown", []PluginScope{"cluster"}},
		{"duplicate", []PluginScope{ScopeGlobal, ScopeGlobal}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := valid
			manifest.Scopes = test.scopes
			if err := manifest.Validate(); err == nil {
				t.Fatalf("accepted scopes %#v", test.scopes)
			}
		})
	}
}

func TestManifestLicenseValidation(t *testing.T) {
	for _, license := range []string{"Apache-2.0", "MIT", "GPL-2.0-only", "MIT OR Apache-2.0", "LicenseRef-Proprietary"} {
		manifest := testManifest("demo", "1.0.0", "shell/bash")
		manifest.Publisher = "community"
		manifest.License = license
		if err := manifest.Validate(); err != nil {
			t.Fatalf("%s: %v", license, err)
		}
	}
	missing := testManifest("demo", "1.0.0", "shell/bash")
	missing.License = ""
	if err := missing.Validate(); err == nil {
		t.Fatal("schema 1 accepted missing license")
	}
	for _, license := range []string{"Apache 2.0", "MIT AND", "not a license"} {
		manifest := testManifest("demo", "1.0.0", "shell/bash")
		manifest.License = license
		if err := manifest.Validate(); err == nil {
			t.Fatalf("accepted %q", license)
		}
	}
}

func TestParseManifestRejectsUnknownFields(t *testing.T) {
	data := `{"schema":1,"id":"bash","name":"Bash","publisher":"mewisme","version":"1.0.0","type":"runtime","provides":["shell/bash"],"permissions":[],"platforms":{"linux/amd64":{"artifact":"bash.tar.gz","sha256":"` + strings.Repeat("a", 64) + `","archive":"tar.gz","entrypoint":"bin/bash"}},"surprise":true}`
	if _, err := ParseManifest([]byte(data)); err == nil {
		t.Fatal("unknown manifest field accepted")
	}
}

func testManifest(id, version string, capability Capability) Manifest {
	return Manifest{
		Schema: ManifestSchema, ID: PluginID(id), Name: id, Publisher: "mewisme", License: "Apache-2.0", Version: Version(version), Type: "runtime",
		Provides: []Capability{capability}, Permissions: []Permission{PermissionProcessExecute},
		Scopes:    []PluginScope{ScopeGlobal},
		Platforms: map[string]PlatformArtifact{"linux/amd64": {Artifact: id + "-" + version + "-linux-amd64.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/" + id}},
	}
}

func testScopedManifest(id, version string, capability Capability, scopes ...PluginScope) Manifest {
	manifest := testManifest(id, version, capability)
	manifest.Scopes = append([]PluginScope(nil), scopes...)
	return manifest
}
