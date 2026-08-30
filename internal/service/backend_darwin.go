//go:build darwin

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type darwinManager struct{}

func NewManager() Manager             { return darwinManager{} }
func (darwinManager) Backend() string { return "launchd" }

func (darwinManager) Install(spec Spec) error {
	path := darwinPlistPath(spec)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(DarwinPlist(spec)), 0644); err != nil {
		return err
	}
	_, _ = runCommand("launchctl", "bootout", darwinTarget(spec))
	_, err := runCommand("launchctl", "bootstrap", darwinDomain(spec), path)
	return err
}

func (darwinManager) Start(spec Spec) error {
	_, err := runCommand("launchctl", "kickstart", "-k", darwinTarget(spec))
	return err
}

func (darwinManager) Stop(spec Spec) error {
	_, err := runCommand("launchctl", "kill", "SIGTERM", darwinTarget(spec))
	return err
}

func (darwinManager) Uninstall(spec Spec) error {
	_, _ = runCommand("launchctl", "bootout", darwinTarget(spec))
	if err := os.Remove(darwinPlistPath(spec)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (darwinManager) Status(spec Spec) (Status, error) {
	output, ok := commandSucceeded("launchctl", "print", darwinTarget(spec))
	status := Status{Installed: ok, Backend: "launchd"}
	if !ok {
		return status, nil
	}
	status.Running = strings.Contains(output, "state = running")
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid = ") {
			status.PID, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid = ")))
			break
		}
	}
	return status, nil
}

func DarwinPlist(spec Spec) string {
	label := darwinLabel(spec)
	args := append([]string{spec.Binary}, Args(spec)...)
	var arguments strings.Builder
	for _, arg := range args {
		arguments.WriteString("\n      <string>")
		arguments.WriteString(xmlText(arg))
		arguments.WriteString("</string>")
	}
	user := ""
	if spec.Scope == ScopeSystem {
		user = "\n    <key>UserName</key>\n    <string>" + xmlText(spec.Account.Username) + "</string>"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>%s
    </array>%s
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>ProcessType</key>
    <string>Background</string>
</dict>
</plist>
`, xmlText(label), arguments.String(), user)
}

func darwinLabel(spec Spec) string { return "me.mewis." + spec.ID }

func darwinPlistPath(spec Spec) string {
	name := darwinLabel(spec) + ".plist"
	if spec.Scope == ScopeSystem {
		return filepath.Join(string(filepath.Separator), "Library", "LaunchDaemons", name)
	}
	return filepath.Join(spec.Account.HomeDir, "Library", "LaunchAgents", name)
}

func darwinDomain(spec Spec) string {
	if spec.Scope == ScopeSystem {
		return "system"
	}
	return "gui/" + spec.Account.UID
}

func darwinTarget(spec Spec) string { return darwinDomain(spec) + "/" + darwinLabel(spec) }

func xmlText(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	return strings.ReplaceAll(value, ">", "&gt;")
}
