package application

import (
	"context"
	"errors"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

func TestLookupTunnelProviderMissing(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	_, err := LookupTunnelProvider("missing")
	var missing ProviderNotInstalledError
	if !errors.As(err, &missing) || missing.Provider != "missing" {
		t.Fatalf("err = %v", err)
	}
}

func TestStartTunnelProviderUnknownTarget(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	err := StartTunnelProvider(context.Background(), config.Default(), "cf", "ssh")
	if !errors.Is(err, tunnelprovider.ErrInvalidDescriptor) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartTunnelProviderAuthFailsBeforeEnable(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	if err := StartTunnelProvider(context.Background(), cfg, "cf", "mcp"); !errors.Is(err, tunnelprovider.ErrMCPTokenMissing) {
		t.Fatalf("err = %v", err)
	}
	ref, err := LookupTunnelProvider("cf")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Enabled {
		t.Fatal("provider enabled after failed start")
	}
}
