package cftunnel

import (
	"context"
	"encoding/json"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/plugins/cf-tunnel/internal/cloudflared"
)

func TestHandlerDescribeAndStartOrigin(t *testing.T) {
	h := &Handler{Manager: newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		return stubTunnel(ctx, cfg), nil
	})}
	desc, err := h.Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := desc.(runtimeplugin.DescribeResult)
	if result.Provider != "cf" || len(result.Targets) != 2 {
		t.Fatalf("describe = %#v", result)
	}
	raw, err := json.Marshal(runtimeplugin.StartParams{Target: TargetMCP, Origin: "http://127.0.0.1:37421", PublicPath: "/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Start(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	st := waitTarget(t, h.Manager, TargetMCP, true, "")
	if st.Origin != "http://127.0.0.1:37421" || st.URL == "" {
		t.Fatalf("status = %#v", st)
	}
}
