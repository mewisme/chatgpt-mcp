package plugin

import (
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

func TestManifestPlatformFallsBackToAnyAny(t *testing.T) {
	manifest := Manifest{
		Schema: ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", Version: "1.0.0", Type: "web-ui",
		Provides: []Capability{CapabilityWebUIAdmin}, Permissions: []Permission{},
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
		Schema: ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", Version: "1.0.0", Type: "web-ui",
		Provides: []Capability{CapabilityWebUIAdmin}, Permissions: []Permission{},
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

func TestParseManifestRejectsUnknownFields(t *testing.T) {
	data := `{"schema":1,"id":"bash","name":"Bash","publisher":"mewisme","version":"1.0.0","type":"runtime","provides":["shell/bash"],"permissions":[],"platforms":{"linux/amd64":{"artifact":"bash.tar.gz","sha256":"` + strings.Repeat("a", 64) + `","archive":"tar.gz","entrypoint":"bin/bash"}},"surprise":true}`
	if _, err := ParseManifest([]byte(data)); err == nil {
		t.Fatal("unknown manifest field accepted")
	}
}

func testManifest(id, version string, capability Capability) Manifest {
	return Manifest{
		Schema: ManifestSchema, ID: PluginID(id), Name: id, Publisher: "mewisme", Version: Version(version), Type: "runtime",
		Provides: []Capability{capability}, Permissions: []Permission{PermissionProcessExecute},
		Platforms: map[string]PlatformArtifact{"linux/amd64": {Artifact: id + "-" + version + "-linux-amd64.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/" + id}},
	}
}
