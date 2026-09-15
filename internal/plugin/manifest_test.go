package plugin

import (
	"strings"
	"testing"
)

func TestManifestValidation(t *testing.T) {
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Platform("windows", "amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Platform("linux", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
	compatible, err := manifest.CompatibleWithCore("0.2.24")
	if err != nil || !compatible {
		t.Fatalf("compatibility = %v, %v", compatible, err)
	}
	compatible, err = manifest.CompatibleWithCore("0.2.23")
	if err != nil || compatible {
		t.Fatalf("old core compatibility = %v, %v", compatible, err)
	}
}

func TestManifestRejectsUnsafeMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"id traversal", func(manifest *Manifest) { manifest.ID = "../bash" }},
		{"version traversal", func(manifest *Manifest) { manifest.Version = "../1.0.0" }},
		{"unknown permission", func(manifest *Manifest) { manifest.Permissions = []Permission{"host/root"} }},
		{"unknown capability", func(manifest *Manifest) { manifest.Provides = []Capability{"unknown/bash"} }},
		{"artifact traversal", func(manifest *Manifest) {
			artifact := manifest.Platforms["windows/amd64"]
			artifact.Artifact = "../bash.zip"
			manifest.Platforms["windows/amd64"] = artifact
		}},
		{"entrypoint traversal", func(manifest *Manifest) {
			artifact := manifest.Platforms["windows/amd64"]
			artifact.Entrypoint = "../bash.exe"
			manifest.Platforms["windows/amd64"] = artifact
		}},
		{"windows entrypoint traversal", func(manifest *Manifest) {
			artifact := manifest.Platforms["windows/amd64"]
			artifact.Entrypoint = `C:\\bash.exe`
			manifest.Platforms["windows/amd64"] = artifact
		}},
		{"invalid digest", func(manifest *Manifest) {
			artifact := manifest.Platforms["windows/amd64"]
			artifact.SHA256 = "bad"
			manifest.Platforms["windows/amd64"] = artifact
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := testManifest("bash", "1.0.0", "shell/bash")
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}

func TestParseManifestRejectsUnknownFields(t *testing.T) {
	data := []byte(`{"schema":1,"id":"bash","name":"Bash","publisher":"mewisme","version":"1.0.0","type":"runtime","provides":["shell/bash"],"permissions":[],"platforms":{"windows/amd64":{"artifact":"bash.zip","sha256":"` + strings.Repeat("a", 64) + `","archive":"zip","entrypoint":"usr/bin/bash.exe"}},"surprise":true}`)
	if _, err := ParseManifest(data); err == nil {
		t.Fatal("unknown manifest field accepted")
	}
}

func testManifest(id, version string, capability Capability) Manifest {
	return Manifest{
		Schema: ManifestSchema, ID: PluginID(id), Name: strings.ToUpper(id), Publisher: "mewisme", Version: Version(version), Type: "runtime",
		Requires: Requirements{ChatGPTMCP: ">=0.2.24"}, Provides: []Capability{capability}, Permissions: []Permission{PermissionProcessExecute},
		Platforms: map[string]PlatformArtifact{"windows/amd64": {Artifact: id + "-" + version + "-windows-amd64.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "usr/bin/bash.exe"}},
	}
}
