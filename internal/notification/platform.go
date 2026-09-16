package notification

import (
	"context"
	"errors"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

const (
	notifyAppName  = "chatgpt-mcp"
	notifyTitleEnv = "CGM_NOTIFY_TITLE"
	notifyBodyEnv  = "CGM_NOTIFY_BODY"
)

const darwinNotifyScript = "on run argv\n" +
	"display notification (item 2 of argv) with title (item 1 of argv)\n" +
	"end run\n"

const windowsNotifyScript = `Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$notify = New-Object System.Windows.Forms.NotifyIcon
$notify.Icon = [System.Drawing.SystemIcons]::Information
$notify.Visible = $true
$notify.ShowBalloonTip(8000, $env:CGM_NOTIFY_TITLE, $env:CGM_NOTIFY_BODY, [System.Windows.Forms.ToolTipIcon]::Info)
Start-Sleep -Seconds 8
$notify.Dispose()
`

type platformProvider struct {
	host host
	kind string
}

func PlatformProvider() Provider {
	return newPlatformProvider(defaultHost())
}

func newPlatformProvider(h host) Provider {
	switch strings.ToLower(strings.TrimSpace(h.env(configformat.EnvTesting))) {
	case "1", "true", "yes":
		return UnavailableProvider()
	}
	if h.runningOnWSL() {
		return platformProvider{host: h, kind: "wsl"}
	}
	switch h.goos {
	case "linux":
		return platformProvider{host: h, kind: "linux"}
	case "darwin":
		return platformProvider{host: h, kind: "darwin"}
	case "windows":
		return platformProvider{host: h, kind: "windows"}
	default:
		return UnavailableProvider()
	}
}

func (p platformProvider) Available(context.Context) bool {
	switch p.kind {
	case "linux":
		return p.linuxAvailable()
	case "darwin":
		return p.host.has("osascript")
	case "windows":
		return p.windowsHostAvailable()
	case "wsl":
		return p.windowsHostAvailable() || p.linuxAvailable()
	default:
		return false
	}
}

func (p platformProvider) Capabilities(ctx context.Context) Capabilities {
	if !p.Available(ctx) {
		return Capabilities{}
	}
	caps := Capabilities{Notification: true}
	if p.kind == "linux" || (p.kind == "wsl" && !p.windowsHostAvailable()) {
		if actions, ok := p.linuxServerActions(ctx); ok && !actions {
			caps.Actions = false
		}
	}
	return caps
}

func (p platformProvider) Send(ctx context.Context, note Notification) error {
	if !p.Available(ctx) {
		return errors.New("desktop notifications unavailable")
	}
	switch p.kind {
	case "linux":
		return p.sendLinux(ctx, note)
	case "darwin":
		return p.sendDarwin(ctx, note)
	case "windows":
		return p.sendWindows(ctx, note)
	case "wsl":
		if p.windowsHostAvailable() {
			return p.sendWindows(ctx, note)
		}
		return p.sendLinux(ctx, note)
	default:
		return errors.New("desktop notifications unavailable")
	}
}

func (p platformProvider) linuxAvailable() bool {
	if strings.TrimSpace(p.host.env("DBUS_SESSION_BUS_ADDRESS")) == "" && strings.TrimSpace(p.host.env("XDG_RUNTIME_DIR")) == "" {
		return false
	}
	return p.host.has("gdbus") || p.host.has("notify-send")
}

func (p platformProvider) windowsHostAvailable() bool {
	return p.host.has("powershell.exe") || p.host.has("pwsh.exe")
}

func (p platformProvider) linuxServerActions(ctx context.Context) (bool, bool) {
	path, err := p.host.lookup("gdbus")
	if err != nil {
		return false, false
	}
	out, err := p.host.output(ctx, runSpec{Name: path, Args: []string{
		"call", "--session",
		"--dest", "org.freedesktop.Notifications",
		"--object-path", "/org/freedesktop/Notifications",
		"--method", "org.freedesktop.Notifications.GetCapabilities",
	}})
	if err != nil {
		return false, false
	}
	return strings.Contains(strings.ToLower(string(out)), "actions"), true
}

func (p platformProvider) sendLinux(ctx context.Context, note Notification) error {
	if path, err := p.host.lookup("gdbus"); err == nil {
		return p.host.run(ctx, runSpec{Name: path, Args: linuxNotifyArgs(note.Title, note.Body)})
	}
	path, err := p.host.lookup("notify-send")
	if err != nil {
		return err
	}
	return p.host.run(ctx, runSpec{Name: path, Args: []string{"--app-name=" + notifyAppName, "--", note.Title, note.Body}})
}

func linuxNotifyArgs(title, body string) []string {
	return []string{
		"call", "--session",
		"--dest", "org.freedesktop.Notifications",
		"--object-path", "/org/freedesktop/Notifications",
		"--method", "org.freedesktop.Notifications.Notify",
		notifyAppName, "0", "", title, body, "[]", "{}", "5000",
	}
}

func (p platformProvider) sendDarwin(ctx context.Context, note Notification) error {
	path, err := p.host.lookup("osascript")
	if err != nil {
		return err
	}
	return p.host.run(ctx, runSpec{Name: path, Args: []string{"-", note.Title, note.Body}, Stdin: darwinNotifyScript})
}

func (p platformProvider) sendWindows(ctx context.Context, note Notification) error {
	path, err := p.host.lookup("powershell.exe")
	if err != nil {
		path, err = p.host.lookup("pwsh.exe")
		if err != nil {
			return err
		}
	}
	return p.host.run(ctx, runSpec{
		Name:  path,
		Args:  []string{"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", "-"},
		Env:   []string{notifyTitleEnv + "=" + note.Title, notifyBodyEnv + "=" + note.Body},
		Stdin: windowsNotifyScript,
	})
}
