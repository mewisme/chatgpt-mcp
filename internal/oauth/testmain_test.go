package oauth

import (
	"os"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/secretstore"
)

func TestMain(m *testing.M) {
	cleanup := secretstore.UseMemoryForTesting()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
