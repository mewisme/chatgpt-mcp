package tunnelprovider

import (
	"errors"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

func TestValidateDescriptor(t *testing.T) {
	valid := runtimeplugin.DescribeResult{
		Provider: "cf", Name: "CF Tunnel",
		Targets: []runtimeplugin.Target{
			{ID: "mcp", OriginKind: OriginMCP, RequiresAuthenticatedOrigin: true},
			{ID: "admin", OriginKind: OriginAdmin, RequiresAuthenticatedOrigin: true},
		},
	}
	if err := ValidateDescriptor("cf", valid); err != nil {
		t.Fatal(err)
	}
	mismatch := valid
	mismatch.Provider = "other"
	if err := ValidateDescriptor("cf", mismatch); !errors.Is(err, ErrProviderMismatch) {
		t.Fatalf("err = %v", err)
	}
	dup := valid
	dup.Targets = append(dup.Targets, runtimeplugin.Target{ID: "mcp", OriginKind: OriginMCP})
	if err := ValidateDescriptor("cf", dup); !errors.Is(err, ErrInvalidDescriptor) || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v", err)
	}
	badKind := valid
	badKind.Targets = []runtimeplugin.Target{{ID: "ssh", OriginKind: "tcp"}}
	if err := ValidateDescriptor("cf", badKind); !errors.Is(err, ErrUnsupportedOrigin) {
		t.Fatalf("err = %v", err)
	}
}
