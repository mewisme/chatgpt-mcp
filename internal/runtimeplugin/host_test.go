package runtimeplugin

import (
	"context"
	"testing"
)

func TestHostIsolatesCrashedSession(t *testing.T) {
	bin := buildFixture(t)
	host := NewHost()
	t.Cleanup(func() { host.Close(context.Background()) })
	ok, err := host.Ensure(context.Background(), Spec{ID: "ok", Entrypoint: bin, WorkDir: t.TempDir(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	crash, err := host.Ensure(context.Background(), Spec{ID: "crash", Entrypoint: bin, WorkDir: t.TempDir(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = crash.Call(context.Background(), MethodStart, StartParams{Target: "crash", Origin: "http://127.0.0.1:9"})
	if _, err := ok.Call(context.Background(), MethodDescribe, nil); err != nil {
		t.Fatalf("healthy session affected: %v", err)
	}
}
