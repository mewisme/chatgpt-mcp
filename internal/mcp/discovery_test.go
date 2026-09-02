package mcp

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
)

func TestServerInstructionsUseSharedInstructionGuidance(t *testing.T) {
	if ServerInstructions != instructioncontext.StaticServerInstructions() {
		t.Fatalf("server instructions drifted from shared guidance")
	}
}
