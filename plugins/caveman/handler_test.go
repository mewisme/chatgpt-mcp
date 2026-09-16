package caveman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/toolprovider"
)

func TestHandlerTurnRespectsReconciledMode(t *testing.T) {
	handler := NewHandler()
	if _, err := handler.Reconcile(context.Background(), mustJSON(t, toolprovider.ReconcileParams{Settings: map[string]any{"default_active": true, "default_mode": "wenyan-ultra"}})); err != nil {
		t.Fatal(err)
	}
	raw, err := handler.Invoke(context.Background(), mustJSON(t, toolprovider.InvokeParams{Tool: "caveman_turn", Args: map[string]any{"workspace_id": "ws_test", "prompt": "continue", "action": "refresh"}}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(raw)
	if !strings.Contains(string(data), `"mode":"wenyan-ultra"`) || !strings.Contains(string(data), "CAVEMAN MODE ACTIVE") {
		t.Fatalf("got %s", data)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
