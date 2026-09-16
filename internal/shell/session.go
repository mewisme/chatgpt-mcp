package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	statepkg "go.mewis.me/chatgpt-mcp/internal/state"
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
	Command         string `json:"command"`
	CWD             string `json:"cwd"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	ExitCode        int    `json:"exit_code"`
	TimedOut        bool   `json:"timed_out"`
}

type Manager struct {
	workspaces *workspace.Manager
	root       string
	executions *ExecutionHub
	providers  *ProviderResolver
	wrappers   *pluginpkg.CommandWrapperPipeline
	mu         sync.Mutex
	sessions   map[string]*session
	timeout    time.Duration
}

type session struct {
	mu    sync.Mutex
	state SessionState
}

var (
	cdPattern    = regexp.MustCompile(`(?i)^cd(?:\s+(.+?))?(?:\s*;\s*|\s*&&\s*|$)`)
	pushdPattern = regexp.MustCompile(`(?i)^pushd\s+(.+?)(?:\s*;\s*|\s*&&\s*|$)`)
)

func DefaultStateRoot() string {
	return configformat.RootPath()
}

func NewManager(workspaces *workspace.Manager, root string) *Manager {
	return NewManagerWithExecutions(workspaces, root, NewExecutionHub())
}

func NewManagerWithExecutions(workspaces *workspace.Manager, root string, executions *ExecutionHub) *Manager {
	return NewManagerWithProviderResolver(workspaces, root, executions, DefaultProviderResolver())
}

func NewManagerWithProviderResolver(workspaces *workspace.Manager, root string, executions *ExecutionHub, providers *ProviderResolver) *Manager {
	if executions == nil {
		executions = NewExecutionHub()
	}
	if providers == nil {
		providers = DefaultProviderResolver()
	}
	return &Manager{workspaces: workspaces, root: root, executions: executions, providers: providers, wrappers: pluginpkg.NewCommandWrapperPipeline(providers.PluginStore()), sessions: map[string]*session{}, timeout: defaultCommandTimeout}
}

func (m *Manager) Executions() *ExecutionHub {
	if m == nil {
		return nil
	}
	return m.executions
}

func (m *Manager) SetConfiguredExecutable(path string) error {
	if m == nil || m.providers == nil {
		return errors.New("shell provider resolver is unavailable")
	}
	return m.providers.SetConfiguredExecutable(path)
}

func (m *Manager) Provider() (Provider, error) {
	if m == nil || m.providers == nil {
		return Provider{}, errors.New("shell provider resolver is unavailable")
	}
	return m.providers.Resolve()
}

func (m *Manager) SetCommandWrapperPipeline(pipeline *pluginpkg.CommandWrapperPipeline) {
	if m != nil {
		m.wrappers = pipeline
	}
}

func (m *Manager) Status(workspaceID string) (Status, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return Status{}, err
	}
	workspaceID = item.ID
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
	workspaceID = item.ID
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

func (m *Manager) Exec(ctx context.Context, workspaceID, command string) (ExecResult, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return ExecResult{}, err
	}
	workspaceID = item.ID
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return ExecResult{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()

	baseCWD, err := m.resolveDirectory(workspaceID, item.Path, current.state.CWD)
	if err != nil {
		return ExecResult{}, err
	}
	current.state.CWD = baseCWD
	cwd, effective, err := m.applyCWDDirectives(workspaceID, baseCWD, command)
	if err != nil {
		return ExecResult{}, err
	}
	if strings.TrimSpace(effective) == "" {
		effective = "pwd"
	}
	requestedEffective := effective
	plan, err := m.prepareCommand(ctx, "run_command", requestedEffective)
	if err != nil {
		return ExecResult{}, err
	}
	if err := m.workspaces.ValidateShellCommandContext(ctx, workspaceID, cwd, plan.Security); err != nil {
		return ExecResult{}, err
	}
	current.state.CWD = cwd
	current.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	current.state.RecentCommands = append(current.state.RecentCommands, requestedEffective)
	if len(current.state.RecentCommands) > maxHistory {
		current.state.RecentCommands = append([]string(nil), current.state.RecentCommands[len(current.state.RecentCommands)-maxHistory:]...)
	}

	metadata := executionMetadata(ctx)
	source := metadata.Source
	if source == "" {
		source = executionSource(ctx)
	}
	provider, err := m.resolveProvider(ctx)
	if err != nil {
		return ExecResult{}, err
	}
	run := m.executions.Begin(ExecutionInput{
		WorkspaceID: workspaceID, Tool: "run_command", Command: plan.Effective, RequestedCommand: command, EffectiveCommand: plan.Effective, SecurityCommand: plan.Security,
		WrapperCapability: plan.WrapperCapability, WrapperProvider: plan.WrapperProvider, WrapperVersion: plan.WrapperVersion,
		CWD: cwd, Shell: provider.Language, ShellProvider: provider.Label(), ShellProviderVersion: string(provider.Version), Source: source,
		CallID: metadata.CallID, SessionHash: metadata.SessionHash, ReceivedByInstanceID: metadata.ReceivedByInstanceID, ExecutedByInstanceID: metadata.ExecutedByInstanceID,
		ParentExecutionID: metadata.ParentExecutionID, Origin: metadata.Origin, HookDepth: metadata.HookDepth,
	})
	result, err := runOnce(ctx, plan.Effective, cwd, m.timeout, run, mergeExecutablePath(plan.WrapperPath, provider.Path, m.workspaces.ShellPath()), provider)
	if saveErr := m.save(current.state); saveErr != nil && err == nil {
		return ExecResult{}, saveErr
	}
	return result, err
}

func (m *Manager) ValidateBackgroundCommand(ctx context.Context, workspaceID, command string) (string, error) {
	cwd, _, err := m.prepareBackgroundCommand(ctx, workspaceID, command)
	return cwd, err
}

func (m *Manager) prepareBackgroundCommand(ctx context.Context, workspaceID, command string) (string, commandPlan, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return "", commandPlan{}, err
	}
	workspaceID = item.ID
	current, err := m.session(workspaceID, item.Path)
	if err != nil {
		return "", commandPlan{}, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()

	cwd, err := m.resolveDirectory(workspaceID, item.Path, current.state.CWD)
	if err != nil {
		return "", commandPlan{}, err
	}
	current.state.CWD = cwd
	effectiveCWD, effective, err := m.applyCWDDirectives(workspaceID, cwd, command)
	if err != nil {
		return "", commandPlan{}, err
	}
	if strings.TrimSpace(effective) != strings.TrimSpace(command) || filepath.Clean(effectiveCWD) != filepath.Clean(cwd) {
		return "", commandPlan{}, errors.New("background process command must not contain cwd-changing directives; change the shell cwd first")
	}
	plan, err := m.prepareCommand(ctx, "start_process", effective)
	if err != nil {
		return "", commandPlan{}, err
	}
	if err := m.workspaces.ValidateShellCommandContext(ctx, workspaceID, cwd, plan.Security); err != nil {
		return "", commandPlan{}, err
	}
	return cwd, plan, nil
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
	path, err := m.statePath(workspaceID)
	if err != nil {
		return SessionState{}, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		var state SessionState
		if configformat.UnmarshalPath(path, data, &state) == nil && state.WorkspaceID == workspaceID && strings.TrimSpace(state.CWD) != "" {
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
	path, err := m.statePath(state.WorkspaceID)
	if err != nil {
		return err
	}
	data, err := configformat.MarshalPath(path, state)
	if err != nil {
		return err
	}
	return statepkg.WriteFileAtomic(path, data, 0600)
}

func (m *Manager) statePath(workspaceID string) (string, error) {
	local, err := m.workspaces.LocalState(workspaceID)
	if err != nil {
		return "", err
	}
	return local.StatePath("shell" + configformat.ExtensionForRoot(m.root)), nil
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
		return "", fmt.Errorf("shell cwd is not a directory: %s", resolved)
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

func runOnce(ctx context.Context, command, cwd string, timeout time.Duration, execution *ExecutionRun, shellPath []string, provider Provider) (ExecResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, err := commandForProvider(runCtx, command, provider)
	if err != nil {
		execution.Finish(ExecutionStatusFailed, nil, false)
		return ExecResult{}, err
	}
	cmd.Dir = cwd
	cmd.Env = shellEnvironment(ctx, shellPath, provider.Executable)
	configureCommandLifecycle(cmd)
	stdout, stderr := &logBuffer{}, &logBuffer{}
	cmd.Stdout = io.MultiWriter(stdout, execution.Writer("stdout"))
	cmd.Stderr = io.MultiWriter(stderr, execution.Writer("stderr"))
	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		execution.Finish(ExecutionStatusCancelled, nil, false)
		return ExecResult{}, ctxErr
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		execution.Finish(ExecutionStatusTimedOut, nil, true)
		return ExecResult{}, fmt.Errorf("command timed out after %s", timeout)
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			execution.Finish(ExecutionStatusFailed, nil, false)
			return ExecResult{}, err
		}
		exitCode = exitErr.ExitCode()
	}
	status := ExecutionStatusSuccess
	if exitCode != 0 {
		status = ExecutionStatusFailed
	}
	execution.Finish(status, &exitCode, false)
	stdoutText, stdoutTruncated := stdout.snapshot()
	stderrText, stderrTruncated := stderr.snapshot()
	return ExecResult{Command: command, CWD: cwd, Stdout: strings.TrimSpace(stdoutText), Stderr: strings.TrimSpace(stderrText), StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated, ExitCode: exitCode, TimedOut: false}, nil
}

func commandForProvider(ctx context.Context, command string, provider Provider) (*exec.Cmd, error) {
	if granted, ok := controlguard.ApprovalFromContext(ctx); ok {
		if strings.TrimSpace(command) != strings.TrimSpace(granted.Invocation.Command) {
			return nil, errors.New("approved control-plane command does not match shell invocation")
		}
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve approved control-plane executable: %w", err)
		}
		return exec.CommandContext(ctx, executable, granted.Invocation.Args...), nil
	}
	if strings.TrimSpace(provider.Executable) == "" || provider.Language != "bash" {
		return nil, errors.New("bash shell provider is unavailable")
	}
	return exec.CommandContext(ctx, provider.Executable, "--noprofile", "--norc", "-c", command), nil
}

func (m *Manager) resolveProvider(ctx context.Context) (Provider, error) {
	if _, ok := controlguard.ApprovalFromContext(ctx); ok {
		return Provider{}, nil
	}
	if m == nil || m.providers == nil {
		return Provider{}, errors.New("shell provider resolver is unavailable")
	}
	return m.providers.Resolve()
}

func shellMarkdownLanguage(shell string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(shell)))
	switch base {
	case "bash", "bash.exe":
		return "bash"
	case "zsh", "zsh.exe":
		return "zsh"
	case "fish", "fish.exe":
		return "fish"
	case "sh", "sh.exe", "dash", "dash.exe", "ash", "ash.exe":
		return "sh"
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		return "powershell"
	case "cmd", "cmd.exe":
		return "batch"
	default:
		return "shell"
	}
}

func stripQuotes(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
