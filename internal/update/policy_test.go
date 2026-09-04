package update

import (
	"errors"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/install"
)

func TestPolicyForInstallation(t *testing.T) {
	directMetadata := &install.Metadata{Schema: install.MetadataSchema, Method: install.MethodDirect, Version: "v1.0.0", InstallDir: "/managed"}
	tests := []struct {
		name      string
		detection install.Detection
		action    PolicyAction
		command   string
	}{
		{"direct", install.Detection{Method: install.MethodDirect, Metadata: directMetadata}, PolicySelfUpdate, ""},
		{"legacy-direct", install.Detection{Method: install.MethodDirect}, PolicyInstallFirst, "chatgpt-mcp install"},
		{"homebrew", install.Detection{Method: install.MethodHomebrew}, PolicyDelegate, "brew upgrade --cask chatgpt-mcp"},
		{"scoop", install.Detection{Method: install.MethodScoop}, PolicyDelegate, "scoop update chatgpt-mcp"},
		{"go", install.Detection{Method: install.MethodGo}, PolicyUnsupported, ""},
		{"development", install.Detection{Method: install.MethodDevelopment}, PolicyUnsupported, ""},
		{"standalone", install.Detection{Method: install.MethodStandalone}, PolicyInstallFirst, "chatgpt-mcp install"},
		{"unknown", install.Detection{Method: install.MethodUnknown}, PolicyUnsupported, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := PolicyForInstallation(test.detection)
			if policy.Method != test.detection.Method || policy.Action != test.action || policy.Command != test.command {
				t.Fatalf("policy = %+v", policy)
			}
			if test.action == PolicySelfUpdate || test.action == PolicyDelegate {
				if err := policy.Error(); err != nil {
					t.Fatalf("policy error = %v", err)
				}
			} else if err := policy.Error(); !errors.Is(err, ErrSelfUpdateUnavailable) {
				t.Fatalf("policy error = %v", err)
			}
		})
	}
}

func TestPolicyRejectsMismatchedDirectMetadata(t *testing.T) {
	policy := PolicyForInstallation(install.Detection{Method: install.MethodDirect, Metadata: &install.Metadata{Method: install.MethodScoop}})
	if policy.Action != PolicyUnsupported || !errors.Is(policy.Error(), ErrSelfUpdateUnavailable) {
		t.Fatalf("policy = %+v, error = %v", policy, policy.Error())
	}
}
