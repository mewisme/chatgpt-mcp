package notification

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func testHost(goos string, env map[string]string, tools map[string]string) host {
	return host{
		goos:   goos,
		getenv: func(key string) string { return env[key] },
		lookPath: func(name string) (string, error) {
			if path, ok := tools[name]; ok {
				return path, nil
			}
			return "", errors.New("not found")
		},
		readFile: func(string) ([]byte, error) { return nil, errors.New("missing") },
	}
}

func TestLinuxHeadlessIsUnavailable(t *testing.T) {
	provider := newPlatformProvider(testHost("linux", nil, map[string]string{"gdbus": "/usr/bin/gdbus"}))
	if provider.Available(context.Background()) {
		t.Fatal("headless linux reported desktop notifications")
	}
}

func TestLinuxRequiresSessionBus(t *testing.T) {
	provider := newPlatformProvider(testHost("linux", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus"}, nil))
	if provider.Available(context.Background()) {
		t.Fatal("linux without notify tools reported available")
	}
}

func TestLinuxGdbusSendIsPassive(t *testing.T) {
	var ran runSpec
	h := testHost("linux", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus"}, map[string]string{"gdbus": "/usr/bin/gdbus"})
	h.outputFn = func(context.Context, runSpec) ([]byte, error) {
		return []byte("(['body'],)"), nil
	}
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	caps := provider.Capabilities(context.Background())
	if !caps.Notification || caps.Actions {
		t.Fatalf("caps=%#v", caps)
	}
	if err := provider.Send(context.Background(), Notification{Title: "ChatGPT MCP approval requested", Body: "demo · Allow git\nOpen CGM to review"}); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/usr/bin/gdbus" || strings.Contains(strings.Join(ran.Args, " "), "Review") || strings.Contains(strings.Join(ran.Args, " "), "approve") {
		t.Fatalf("send spec=%#v", ran)
	}
	joined := strings.Join(ran.Args, "\x00")
	if !strings.Contains(joined, "ChatGPT MCP approval requested") || !strings.Contains(joined, "[]") {
		t.Fatalf("missing title or empty actions: %#v", ran.Args)
	}
}

func TestLinuxIgnoresServerActionsWithoutListener(t *testing.T) {
	var ran runSpec
	h := testHost("linux", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus"}, map[string]string{"gdbus": "/usr/bin/gdbus"})
	h.outputFn = func(context.Context, runSpec) ([]byte, error) {
		return []byte("(['body', 'actions', 'icon-static'],)"), nil
	}
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	caps := provider.Capabilities(context.Background())
	if caps.Actions {
		t.Fatal("v1 must not claim Review actions without a D-Bus listener")
	}
	if err := provider.Send(context.Background(), Notification{Title: "title", Body: "body", OpenAction: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(ran.Args, " "), "Review") {
		t.Fatalf("actions leaked into notify: %#v", ran.Args)
	}
}

func TestLinuxNotifySendFallbackOmitsActions(t *testing.T) {
	var ran runSpec
	h := testHost("linux", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, map[string]string{"notify-send": "/usr/bin/notify-send"})
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	if err := provider.Send(context.Background(), Notification{Title: "--title", Body: "body; rm -rf /"}); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/usr/bin/notify-send" || strings.Join(ran.Args, " ") != "--app-name=chatgpt-mcp -- --title body; rm -rf /" {
		t.Fatalf("notify-send spec=%#v", ran)
	}
}

func TestWSLDoesNotRequireLinuxDBus(t *testing.T) {
	h := testHost("linux", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, map[string]string{"powershell.exe": "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"})
	provider := newPlatformProvider(h)
	if !provider.Available(context.Background()) {
		t.Fatal("WSL host notifications unavailable without D-Bus")
	}
	caps := provider.Capabilities(context.Background())
	if !caps.Notification || caps.Actions {
		t.Fatalf("wsl caps=%#v", caps)
	}
}

func TestWSLWithoutHostOrDesktopIsUnavailable(t *testing.T) {
	provider := newPlatformProvider(testHost("linux", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, nil))
	if provider.Available(context.Background()) {
		t.Fatal("WSL without host tools should not require D-Bus alone")
	}
}

func TestWSLPrefersWindowsHostSend(t *testing.T) {
	var ran runSpec
	h := testHost("linux", map[string]string{
		"WSL_DISTRO_NAME":          "Ubuntu",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus",
	}, map[string]string{"powershell.exe": "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe", "gdbus": "/usr/bin/gdbus"})
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	if err := provider.Send(context.Background(), Notification{Title: "title", Body: "body"}); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe" || ran.Stdin != windowsNotifyScript {
		t.Fatalf("wsl send used %#v", ran)
	}
}

func TestDarwinSendUsesArgv(t *testing.T) {
	var ran runSpec
	h := testHost("darwin", nil, map[string]string{"osascript": "/usr/bin/osascript"})
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	if err := provider.Send(context.Background(), Notification{Title: `say "hi"`, Body: "body"}); err != nil {
		t.Fatal(err)
	}
	if ran.Name != "/usr/bin/osascript" || strings.Join(ran.Args, " ") != `- say "hi" body` || ran.Stdin != darwinNotifyScript {
		t.Fatalf("osascript spec=%#v", ran)
	}
}

func TestWindowsSendPassesBodyThroughEnv(t *testing.T) {
	var ran runSpec
	h := testHost("windows", nil, map[string]string{"powershell.exe": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`})
	h.runFn = func(_ context.Context, spec runSpec) error {
		ran = spec
		return nil
	}
	provider := newPlatformProvider(h)
	if err := provider.Send(context.Background(), Notification{Title: "title", Body: "body & whoami"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(ran.Env, "\n"), "CGM_NOTIFY_BODY=body & whoami") || ran.Stdin != windowsNotifyScript {
		t.Fatalf("powershell spec=%#v", ran)
	}
}

func TestPlatformProviderNeverRequired(t *testing.T) {
	provider := newPlatformProvider(testHost("plan9", nil, nil))
	if provider.Available(context.Background()) {
		t.Fatal("unknown platform reported available")
	}
	if err := provider.Send(context.Background(), Notification{Title: "t", Body: "b"}); err == nil {
		t.Fatal("expected send failure")
	}
}

func TestTestingEnvDisablesPlatformProvider(t *testing.T) {
	provider := newPlatformProvider(testHost("linux", map[string]string{
		configformat.EnvTesting:    "1",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/bus",
	}, map[string]string{"gdbus": "/usr/bin/gdbus"}))
	if provider.Available(context.Background()) {
		t.Fatal("testing env still reported desktop notifications")
	}
}
