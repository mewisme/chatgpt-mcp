package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const (
	maxHistory            = 50
	defaultCommandTimeout = 120 * time.Second
)

type SessionState struct {
	WorkspaceID    string   `json:"workspace_id"`
	CWD            string   `json:"cwd"`
	StartedAt      string   `json:"started_at"`
	UpdatedAt      string   `json:"updated_at"`
	RecentCommands []string `json:"recent_commands"`
}

type Status struct {
	Active         bool     `json:"active"`
	CWD            string   `json:"cwd"`
	StartedAt      string   `json:"started_at"`
	RecentCommands []string `json:"recent_commands"`
}

type ExecResult struct {
	Command  string `json:"command"`
	CWD      string `json:"cwd"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

type Manager struct {
	workspaces *workspace.Manager
	root       string
	mu         sync.Mutex
	sessions   map[string]*session
	timeout    time.Duration
}

type session struct {
	mu    sync.Mutex
	state SessionState
}

var (
	setLocationPattern = regexp.MustCompile(`(?i)^(?:Set-Location|sl)\s+(.+?)(?:\s*;\s*|\s*&&\s*|$)`)
	cdPattern          = regexp.MustCompile(`(?i)^cd(?:\s+(.+?))?(?:\s*;\s*|\s*&&\s*|$)`)
	pushdPattern       = regexp.MustCompile(`(?i)^pushd\s+(.+?)(?:\s*;\s*|\s*&&\s*|$)`)
)

func DefaultStateRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".chatgpt-mcp"
	}
	return filepath.Join(home, ".config", "chatgpt-mcp")
}

func NewManager(workspaces *workspace.Manager, root string) *Manager {
	return &Manager{workspaces: workspaces, root: root, sessions: map[string]*session{}, timeout: defaultCommandTimeout}
}

func (m *Manager) Status(workspaceID string) (Status, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return Status{}, err
	}
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return Status{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	return statusFromState(current.state), nil
}

func (m *Manager) Reset(workspaceID, path string) (Status, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return Status{}, err
	}
	target := item.Path
	if strings.TrimSpace(path) != "" {
		target, err = m.workspaces.ResolvePath(workspaceID, item.Path, path, true)
		if err != nil {
			return Status{}, err
		}
		info, err := os.Stat(target)
		if err != nil {
			return Status{}, err
		}
		if !info.IsDir() {
			return Status{}, fmt.Errorf("shell reset path is not a directory: %s", target)
		}
	}
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return Status{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	current.state = SessionState{WorkspaceID: workspaceID, CWD: target, StartedAt: now, UpdatedAt: now, RecentCommands: []string{}}
	if err := m.save(current.state); err != nil {
		return Status{}, err
	}
	return statusFromState(current.state), nil
}

func (m *Manager) Exec(ctx context.Context, workspaceID, workingDirectory, command string) (ExecResult, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return ExecResult{}, err
	}
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return ExecResult{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()

	mutation := m.workspaces.IsMutationCommand(command)
	baseCWD := current.state.CWD

	if strings.TrimSpace(workingDirectory) != "" {
		resolved, err := m.resolveDirectory(workspaceID, item.Path, workingDirectory)
		if err != nil {
			return ExecResult{}, err
		}
		if mutation && filepath.Clean(current.state.CWD) != filepath.Clean(resolved) {
			return ExecResult{}, fmt.Errorf("mutation command denied: persisted cwd %s does not match working_directory %s", current.state.CWD, resolved)
		}
		baseCWD = resolved
	} else if mutation {
		return ExecResult{}, errors.New("mutation command denied: working_directory is required for mutating commands")
	}

	if err := m.workspaces.ValidateShellCommand(workspaceID, baseCWD, command); err != nil {
		return ExecResult{}, err
	}

	cwd, effective, err := m.applyCWDDirectives(workspaceID, baseCWD, command)
	if err != nil {
		return ExecResult{}, err
	}
	if mutation && filepath.Clean(cwd) != filepath.Clean(baseCWD) {
		return ExecResult{}, fmt.Errorf("mutation command denied: effective cwd %s does not match working_directory %s", cwd, baseCWD)
	}

	if strings.TrimSpace(effective) == "" {
		effective = pwdCommand()
	}
	current.state.CWD = cwd
	current.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	current.state.RecentCommands = append(current.state.RecentCommands, effective)
	if len(current.state.RecentCommands) > maxHistory {
		current.state.RecentCommands = append([]string(nil), current.state.RecentCommands[len(current.state.RecentCommands)-maxHistory:]...)
	}

	result, err := runOnce(ctx, effective, cwd, m.timeout)
	if saveErr := m.save(current.state); saveErr != nil && err == nil {
		return ExecResult{}, saveErr
	}
	return result, err
}

func (m *Manager) ValidateBackgroundCommand(workspaceID, workingDirectory, command string) (string, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return "", err
	}
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return "", err
	}
	current.mu.Lock()
	defer current.mu.Unlock()

	mutation := m.workspaces.IsMutationCommand(command)
	cwd := current.state.CWD
	if strings.TrimSpace(workingDirectory) != "" {
		resolved, err := m.resolveDirectory(workspaceID, item.Path, workingDirectory)
		if err != nil {
			return "", err
		}
		if mutation && filepath.Clean(current.state.CWD) != filepath.Clean(resolved) {
			return "", fmt.Errorf("mutation command denied: persisted cwd %s does not match working_directory %s", current.state.CWD, resolved)
		}
		cwd = resolved
	} else if mutation {
		return "", errors.New("mutation command denied: working_directory is required for mutating commands")
	}
	if err := m.workspaces.ValidateShellCommand(workspaceID, cwd, command); err != nil {
		return "", err
	}
	effectiveCWD, effective, err := m.applyCWDDirectives(workspaceID, cwd, command)
	if err != nil {
		return "", err
	}
	if mutation && filepath.Clean(effectiveCWD) != filepath.Clean(cwd) {
		return "", fmt.Errorf("mutation command denied: effective cwd %s does not match working_directory %s", effectiveCWD, cwd)
	}
	if strings.TrimSpace(effective) != strings.TrimSpace(command) || filepath.Clean(effectiveCWD) != filepath.Clean(cwd) {
		return "", errors.New("background process command must not contain cwd-changing directives; use working_directory instead")
	}
	return cwd, nil
}

func (m *Manager) session(workspaceID, workspaceRoot string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.sessions[workspaceID]; existing != nil {
		return existing, nil
	}
	state, err := m.load(workspaceID, workspaceRoot)
	if err != nil {
		return nil, err
	}
	value := &session{state: state}
	m.sessions[workspaceID] = value
	return value, nil
}

func (m *Manager) load(workspaceID, workspaceRoot string) (SessionState, error) {
	data, err := os.ReadFile(m.statePath(workspaceID))
	if err == nil {
		var state SessionState
		if json.Unmarshal(data, &state) == nil && state.WorkspaceID == workspaceID && strings.TrimSpace(state.CWD) != "" {
			resolved, resolveErr := m.resolveDirectory(workspaceID, workspaceRoot, state.CWD)
			if resolveErr == nil {
				state.CWD = resolved
				if state.RecentCommands == nil {
					state.RecentCommands = []string{}
				}
				if len(state.RecentCommands) > maxHistory {
					state.RecentCommands = append([]string(nil), state.RecentCommands[len(state.RecentCommands)-maxHistory:]...)
				}
				return state, nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return SessionState{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	state := SessionState{WorkspaceID: workspaceID, CWD: workspaceRoot, StartedAt: now, UpdatedAt: now, RecentCommands: []string{}}
	if err := m.save(state); err != nil {
		return SessionState{}, err
	}
	return state, nil
}

func (m *Manager) save(state SessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := m.statePath(state.WorkspaceID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

func (m *Manager) statePath(workspaceID string) string {
	return filepath.Join(m.root, "workspaces", workspaceID, "shell.json")
}

func (m *Manager) resolveDirectory(workspaceID, workspaceRoot, input string) (string, error) {
	resolved, err := m.workspaces.ResolvePath(workspaceID, workspaceRoot, input, true)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working_directory is not a directory: %s", resolved)
	}
	return resolved, nil
}

func (m *Manager) applyCWDDirectives(workspaceID, currentCWD, command string) (string, string, error) {
	cwd := currentCWD
	rest := strings.TrimSpace(command)
	for i := 0; i < 8; i++ {
		var target string
		var matched string
		switch {
		case setLocationPattern.MatchString(rest):
			match := setLocationPattern.FindStringSubmatch(rest)
			target = match[1]
			matched = match[0]
		case cdPattern.MatchString(rest):
			match := cdPattern.FindStringSubmatch(rest)
			if len(match) > 1 {
				target = match[1]
			}
			matched = match[0]
		case pushdPattern.MatchString(rest):
			match := pushdPattern.FindStringSubmatch(rest)
			target = match[1]
			matched = match[0]
		default:
			return cwd, rest, nil
		}
		target = stripQuotes(target)
		if target != "" && target != "-" && target != "~" {
			resolved, err := m.workspaces.ResolvePath(workspaceID, cwd, target, true)
			if err != nil {
				return "", "", err
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return "", "", err
			}
			if !info.IsDir() {
				return "", "", fmt.Errorf("cwd target is not a directory: %s", resolved)
			}
			cwd = resolved
		}
		rest = strings.TrimSpace(strings.TrimPrefix(rest, matched))
	}
	return cwd, rest, nil
}

func statusFromState(state SessionState) Status {
	recent := append([]string(nil), state.RecentCommands...)
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	return Status{Active: true, CWD: state.CWD, StartedAt: state.StartedAt, RecentCommands: recent}
}

func runOnce(ctx context.Context, command, cwd string, timeout time.Duration) (ExecResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, err := commandForPlatform(runCtx, command)
	if err != nil {
		return ExecResult{}, err
	}
	cmd.Dir = cwd
	cmd.Env = shellEnvironment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return ExecResult{}, fmt.Errorf("command timed out after %s", timeout)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ExecResult{}, ctxErr
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return ExecResult{}, err
		}
		exitCode = exitErr.ExitCode()
	}
	return ExecResult{
		Command: command, CWD: cwd, Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String()),
		ExitCode: exitCode, TimedOut: false,
	}, nil
}

func commandForPlatform(ctx context.Context, command string) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" {
		shell, isPwsh, err := windowsShell()
		if err != nil {
			return nil, err
		}
		effective := command
		if !isPwsh {
			effective = transpileCompoundOperators(command)
		}
		return exec.CommandContext(ctx, shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", effective), nil
	}
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		if found, err := exec.LookPath("bash"); err == nil {
			shell = found
		} else {
			shell = "/bin/sh"
		}
	}
	return exec.CommandContext(ctx, shell, "-lc", command), nil
}

func windowsShell() (string, bool, error) {
	configured := strings.TrimSpace(os.Getenv("SHELL"))
	if configured != "" {
		base := strings.ToLower(filepath.Base(configured))
		if base == "pwsh" || base == "pwsh.exe" {
			return configured, true, nil
		}
		if base == "powershell" || base == "powershell.exe" {
			return configured, false, nil
		}
	}
	if shell, err := exec.LookPath("pwsh"); err == nil {
		return shell, true, nil
	}
	if shell, err := exec.LookPath("powershell"); err == nil {
		return shell, false, nil
	}
	return "", false, errors.New("no PowerShell runtime found")
}

func transpileCompoundOperators(command string) string {
	if !strings.Contains(command, "&&") && !strings.Contains(command, "||") {
		return command
	}
	type token struct {
		kind  string
		value string
	}
	tokens := make([]token, 0)
	var current strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(command); {
		char := command[i]
		if char == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteByte(char)
			i++
			continue
		}
		if char == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteByte(char)
			i++
			continue
		}
		if !inSingle && !inDouble && i+1 < len(command) {
			op := command[i : i+2]
			if op == "&&" || op == "||" {
				if text := strings.TrimSpace(current.String()); text != "" {
					tokens = append(tokens, token{kind: "text", value: text})
				}
				tokens = append(tokens, token{kind: op, value: op})
				current.Reset()
				i += 2
				continue
			}
		}
		current.WriteByte(char)
		i++
	}
	if text := strings.TrimSpace(current.String()); text != "" {
		tokens = append(tokens, token{kind: "text", value: text})
	}
	if len(tokens) <= 1 {
		return command
	}
	result := tokens[0].value + "; $__chatgptMcpSuccess = $?"
	for i := 1; i+1 < len(tokens); i += 2 {
		next := tokens[i+1].value
		if tokens[i].kind == "&&" {
			result += "; if ($__chatgptMcpSuccess) { " + next + "; $__chatgptMcpSuccess = $? }"
		} else {
			result += "; if (-not $__chatgptMcpSuccess) { " + next + "; $__chatgptMcpSuccess = $? }"
		}
	}
	return result
}

func shellEnvironment() []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		if index := strings.IndexByte(entry, '='); index >= 0 {
			values[entry[:index]] = entry[index+1:]
		}
	}
	values["CI"] = "true"
	values["PAGER"] = "cat"
	values["GIT_PAGER"] = "cat"
	values["NO_COLOR"] = "1"
	values["npm_config_yes"] = "true"
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func stripQuotes(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func pwdCommand() string {
	if runtime.GOOS == "windows" {
		return "(Get-Location).Path"
	}
	return "pwd"
}
