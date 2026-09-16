package licenseinventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyLicenseTexts(t *testing.T) {
	tests := map[string]string{
		"Apache-2.0":   "Apache License\nVersion 2.0, January 2004",
		"MIT":          "Permission is hereby granted, free of charge, to any person obtaining a copy of this software\nTHE SOFTWARE IS PROVIDED \"AS IS\"",
		"MPL-2.0":      "Mozilla Public License Version 2.0",
		"OFL-1.1":      "SIL OPEN FONT LICENSE Version 1.1",
		"GPL-2.0-only": "GNU GENERAL PUBLIC LICENSE\nVersion 2, June 1991",
		"BSD-3-Clause": "Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:\nNeither the name of the copyright holder nor the names of its contributors may be used",
		"BSD-2-Clause": "Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:\nRedistributions of source code must retain",
	}
	for want, text := range tests {
		if got := classifyLicense(text); got != want {
			t.Fatalf("classify %s: got %q", want, got)
		}
	}
	if got := classifyLicense("SPDX-License-Identifier: MIT OR Apache-2.0"); got != "MIT OR Apache-2.0" {
		t.Fatalf("spdx header = %q", got)
	}
}

func TestPolicyRejectsUnknownAndArtifactScopedLicenses(t *testing.T) {
	policy := Policy{Allowed: []AllowedLicense{
		{SPDX: "Apache-2.0", Kind: "permissive"},
		{SPDX: "GPL-2.0-only", Kind: "copyleft", Rationale: "bash payload", Artifacts: []string{"bash"}},
	}}
	if err := Validate(Inventory{Artifact: "core", Packages: []Package{{Name: "demo", Version: "1", License: "MIT"}}}, policy); err == nil {
		t.Fatal("unknown license accepted")
	}
	if err := Validate(Inventory{Artifact: "core", Packages: []Package{{Name: "demo", Version: "1", License: "GPL-2.0-only"}}}, policy); err == nil {
		t.Fatal("artifact-scoped GPL accepted on core")
	}
	if err := Validate(Inventory{Artifact: "bash", Packages: []Package{{Name: "demo", Version: "1", License: "GPL-2.0-only"}}}, policy); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Inventory{Artifact: "core", Packages: []Package{{Name: "demo", Version: "1"}}}, policy); err == nil {
		t.Fatal("missing license accepted")
	}
}

func TestGenerateWritesStableInventoryWithoutCachePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "LICENSE"), []byte("MIT License"), 0600); err != nil {
		t.Fatal(err)
	}
	collector := Collector{
		Root: root,
		GoList: func(string, []string) ([]Package, error) {
			return []Package{{Name: "example.com/mod", Version: "v1.2.3", License: "MIT", Path: "example.com/mod", Kind: KindGoModule, Text: "MIT License"}}, nil
		},
	}
	inv, err := collector.Generate("rtk")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Artifact != "rtk" || len(inv.Packages) != 1 {
		t.Fatalf("inventory = %#v", inv)
	}
	if inv.Packages[0].Name != "rtk" || inv.Packages[0].License != "Apache-2.0" {
		t.Fatalf("root package = %#v", inv.Packages[0])
	}
	core := Collector{
		Root: root,
		GoList: func(string, []string) ([]Package, error) {
			return []Package{{Name: "example.com/mod", Version: "v1.2.3", License: "MIT", Path: "example.com/mod", Source: "https://example.com/mod", Kind: KindGoModule, Text: "MIT License"}}, nil
		},
	}
	inv, err = core.Generate("core")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(inv, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "license-inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), root) || strings.Contains(string(data), "pkg/mod") {
		t.Fatalf("inventory pinned a cache path: %s", data)
	}
	licenses, err := os.ReadFile(filepath.Join(out, "licenses.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(licenses), "example.com/mod v1.2.3") || !strings.Contains(string(licenses), "SPDX: MIT") {
		t.Fatalf("licenses.txt = %s", licenses)
	}
	notice, err := os.ReadFile(filepath.Join(out, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notice), "example.com/mod v1.2.3 (MIT)") || strings.Contains(string(notice), root) {
		t.Fatalf("NOTICE = %s", notice)
	}
	sbom, err := os.ReadFile(filepath.Join(out, "sbom.spdx.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sbom), `"spdxVersion": "SPDX-2.3"`) || !strings.Contains(string(sbom), `"created": "2026-01-01T00:00:00Z"`) || strings.Contains(string(sbom), "pkg/mod") {
		t.Fatalf("sbom = %s", sbom)
	}
}

func TestRepositoryPolicyCoversGoArtifacts(t *testing.T) {
	root := moduleRoot(t)
	policy, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	collector := Collector{Root: root}
	for _, artifact := range Artifacts() {
		if artifact.NPMRoot != "" {
			if _, err := os.Stat(filepath.Join(root, artifact.NPMRoot, "node_modules")); err != nil {
				t.Logf("skip %s: npm dependencies are not installed", artifact.ID)
				continue
			}
		}
		inv, err := collector.Generate(artifact.ID)
		if err != nil {
			t.Fatalf("%s: %v", artifact.ID, err)
		}
		if err := Validate(inv, policy); err != nil {
			t.Fatalf("%s: %v", artifact.ID, err)
		}
		if inv.Artifact != artifact.ID || len(inv.Packages) == 0 {
			t.Fatalf("%s inventory = %#v", artifact.ID, inv)
		}
		if artifact.ID == "core" {
			for _, pkg := range inv.Packages {
				if strings.Contains(pkg.Path, "third_party/") || strings.Contains(pkg.Path, "cloudflared") || pkg.Name == "ponytail" || pkg.Name == "caveman" {
					t.Fatalf("core inventory includes plugin material %#v", pkg)
				}
			}
		}
		licenses := map[string]struct{}{}
		for _, pkg := range inv.Packages {
			licenses[pkg.License] = struct{}{}
		}
		t.Logf("%s packages=%d licenses=%d", artifact.ID, len(inv.Packages), len(licenses))
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestPonytailNoticeIncludesUpstreamAttribution(t *testing.T) {
	root := moduleRoot(t)
	inv, err := Collector{Root: root, GoList: func(string, []string) ([]Package, error) { return nil, nil }}.Generate("ponytail")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Write(inv, out); err != nil {
		t.Fatal(err)
	}
	notice, err := os.ReadFile(filepath.Join(out, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notice), "Dietrich Gebert") || !strings.Contains(string(notice), "third_party/ponytail") {
		t.Fatalf("NOTICE = %s", notice)
	}
}

func TestLoadPolicyRequiresRationaleForCopyleft(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "licenses"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "licenses", "policy.json"), []byte(`{"allowed":[{"spdx":"GPL-3.0-only","kind":"copyleft"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(Inventory{Artifact: "core", Packages: []Package{{Name: "x", Version: "1", License: "GPL-3.0-only"}}}, policy); err == nil {
		t.Fatal("copyleft without rationale accepted")
	}
}
