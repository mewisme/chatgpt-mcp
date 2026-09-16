package securemcptunnel

import (
	"context"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

func TestHandlerDescribe(t *testing.T) {
	desc, err := NewHandler().Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := desc.(runtimeplugin.DescribeResult)
	if result.Provider != tunnelprovider.ProviderSecureMCP || len(result.Targets) != 1 || result.Targets[0].OriginKind != tunnelprovider.OriginPrivate {
		t.Fatalf("describe = %#v", result)
	}
}
