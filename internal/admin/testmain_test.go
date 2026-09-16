package admin

import (
	"fmt"
	"os"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestMain(m *testing.M) {
	pluginhost.Install()
	_, cleanup, err := testutil.IsolateConfigHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
