package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Event string

const (
	SessionStart     Event = "session_start"
	UserPromptSubmit Event = "user_prompt_submit"
	SubagentStart    Event = "subagent_start"
)

type Hook struct {
	ID            string `json:"id"`
	Plugin        string `json:"plugin"`
	Event         Event  `json:"event"`
	Command       string `json:"command"`
	TimeoutMS     int    `json:"timeout_ms"`
	StatusMessage string `json:"status_message,omitempty"`
	SourcePath    string `json:"source_path"`
	PluginRoot    string `json:"plugin_root"`
	Trusted       bool   `json:"trusted"`
	Supported     bool   `json:"supported"`
	Enabled       bool   `json:"enabled"`
}

type manifest struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

type hookGroup struct {
	Hooks []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout"`
	StatusMessage string `json:"statusMessage"`
}

func CodexHome() string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

func Discover() ([]Hook, error) {
	enabledPlugins, trusted, err := readCodexConfig()
	if err != nil {
		return nil, err
	}
	result := make([]Hook, 0)
	for _, plugin := range enabledPlugins {
		root, err := latestPluginRoot(plugin)
		if err != nil || root == "" {
			continue
		}
		hooksDir := filepath.Join(root, "hooks")
		entries, err := os.ReadDir(hooksDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") || !strings.Contains(strings.ToLower(entry.Name()), "codex") {
				continue
			}
			sourcePath := filepath.Join(hooksDir, entry.Name())
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				continue
			}
			var value manifest
			if json.Unmarshal(data, &value) != nil {
				continue
			}
			for eventName, groups := range value.Hooks {
				event, ok := eventForName(eventName)
				if !ok {
					continue
				}
				for groupIndex, group := range groups {
					for hookIndex, item := range group.Hooks {
						if item.Type != "command" || strings.TrimSpace(item.Command) == "" {
							continue
						}
						id := plugin + ":hooks/" + entry.Name() + ":" + string(event) + ":" + itoa(groupIndex) + ":" + itoa(hookIndex)
						timeout := item.Timeout * 1000
						if timeout == 0 {
							timeout = 5000
						}
						if timeout < 1000 {
							timeout = 1000
						}
						if timeout > 15000 {
							timeout = 15000
						}
						isTrusted := trusted[id]
						result = append(result, Hook{
							ID: id, Plugin: plugin, Event: event,
							Command:   strings.ReplaceAll(item.Command, "${CLAUDE_PLUGIN_ROOT}", root),
							TimeoutMS: timeout, StatusMessage: item.StatusMessage,
							SourcePath: sourcePath, PluginRoot: root, Trusted: isTrusted,
							Supported: event == SessionStart, Enabled: isTrusted,
						})
					}
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func Run(ctx context.Context, hook Hook, workspaceRoot, input string) string {
	timeout := time.Duration(hook.TimeoutMS) * time.Millisecond
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shell := "powershell.exe"
		if path, err := exec.LookPath("pwsh"); err == nil {
			shell = path
		}
		cmd = exec.CommandContext(runCtx, shell, "-NoProfile", "-NonInteractive", "-Command", hook.Command)
	} else {
		shell := "bash"
		if path, err := exec.LookPath(shell); err == nil {
			shell = path
		}
		cmd = exec.CommandContext(runCtx, shell, "-lc", hook.Command)
	}
	cmd.Dir = workspaceRoot
	cmd.Env = append(os.Environ(), "PLUGIN_DATA="+CodexHome())
	cmd.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return hookContext(strings.TrimSpace(stdout.String()))
}

func readCodexConfig() ([]string, map[string]bool, error) {
	path := filepath.Join(CodexHome(), "config.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, map[string]bool{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	plugins := map[string]bool{}
	trusted := map[string]bool{}
	currentPlugin := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentPlugin = ""
			if value, ok := quotedSectionValue(line, `plugins."`); ok {
				currentPlugin = value
			}
			if value, ok := quotedSectionValue(line, `hooks.state."`); ok {
				trusted[value] = true
			}
			continue
		}
		if currentPlugin != "" {
			key, value, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(key) == "enabled" && strings.EqualFold(strings.TrimSpace(value), "true") {
				plugins[currentPlugin] = true
			}
		}
	}
	result := make([]string, 0, len(plugins))
	for plugin := range plugins {
		result = append(result, plugin)
	}
	sort.Strings(result)
	return result, trusted, nil
}

func quotedSectionValue(section, prefix string) (string, bool) {
	fullPrefix := "[" + prefix
	if !strings.HasPrefix(section, fullPrefix) || !strings.HasSuffix(section, `"]`) {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(section, fullPrefix), `"]`)
	return value, value != ""
}

func latestPluginRoot(plugin string) (string, error) {
	index := strings.LastIndex(plugin, "@")
	if index <= 0 || index == len(plugin)-1 {
		return "", nil
	}
	packageName := plugin[:index]
	source := plugin[index+1:]
	base := filepath.Join(CodexHome(), "plugins", "cache", source, packageName)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	versions := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))
	if len(versions) == 0 {
		return "", nil
	}
	return filepath.Join(base, versions[0]), nil
}

func eventForName(value string) (Event, bool) {
	switch value {
	case "SessionStart":
		return SessionStart, true
	case "UserPromptSubmit":
		return UserPromptSubmit, true
	case "SubagentStart":
		return SubagentStart, true
	default:
		return "", false
	}
}

func hookContext(output string) string {
	var value struct {
		HookSpecificOutput struct {
			AdditionalContext any `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal([]byte(output), &value) == nil {
		if text, ok := value.HookSpecificOutput.AdditionalContext.(string); ok {
			return text
		}
	}
	return output
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
