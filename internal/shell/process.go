package shell

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const (
	maxProcessLogChars       = 400_000
	maxFinishedProcesses     = 100
	finishedProcessRetention = 24 * time.Hour
)

type ProcessInfo struct {
	ID          string  `json:"id"`
	ExecutionID string  `json:"execution_id,omitempty"`
	PID         int     `json:"pid"`
	Command     string  `json:"command"`
	CWD         string  `json:"cwd"`
	StartedAt   string  `json:"started_at"`
	Running     bool    `json:"running"`
	ExitCode    *int    `json:"exit_code"`
	Signal      *string `json:"signal"`
}

type StartResult struct {
	ID          string `json:"id"`
	ExecutionID string `json:"execution_id,omitempty"`
	PID         int    `json:"pid"`
	Command     string `json:"command"`
	CWD         string `json:"cwd"`
	StartedAt   string `json:"started_at"`
}

type OutputResult struct {
	ID       string  `json:"id"`
	Running  bool    `json:"running"`
	ExitCode *int    `json:"exit_code"`
	Signal   *string `json:"signal"`
	Stdout   string  `json:"stdout"`
	Stderr   string  `json:"stderr"`
}

type StopResult struct {
	ID            string `json:"id"`
	Force         bool   `json:"force,omitempty"`
	AlreadyExited bool   `json:"already_exited,omitempty"`
}

type managedProcess struct {
	mu         sync.Mutex
	workspace  string
	id         string
	command    string
	cwd        string
	startedAt  string
	cmd        *exec.Cmd
	stdout     *logBuffer
	stderr     *logBuffer
	exitCode   *int
	signal     *string
	finishedAt time.Time
	execution  *ExecutionRun
}

type ProcessManager struct {
	workspaces  *workspace.Manager
	shell       *Manager
	mu          sync.RWMutex
	processes   map[string]*managedProcess
	order       []string
	maxFinished int
	retention   time.Duration
	executions  *ExecutionHub
}

type logBuffer struct {
	mu   sync.Mutex
	data []byte
}

func NewProcessManager(workspaces *workspace.Manager, shell *Manager) *ProcessManager {
	return &ProcessManager{workspaces: workspaces, shell: shell, processes: map[string]*managedProcess{}, maxFinished: maxFinishedProcesses, retention: finishedProcessRetention}
}

func NewProcessManagerWithExecutions(workspaces *workspace.Manager, shell *Manager, executions *ExecutionHub) *ProcessManager {
	manager := NewProcessManager(workspaces, shell)
	manager.executions = executions
	return manager
}

func (m *ProcessManager) Start(ctx context.Context, workspaceID, command string) (StartResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspaceItem, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return StartResult{}, err
	}
	workspaceID = workspaceItem.ID
	cwd, err := m.shell.ValidateBackgroundCommand(ctx, workspaceID, command)
	if err != nil {
		return StartResult{}, err
	}
	strict := m.workspaces.ShellApprovalPolicy() == workspace.ShellApprovalStrict
	processCtx := context.WithoutCancel(ctx)
	cmd, err := commandForPlatformPolicy(processCtx, command, strict, m.workspaces.ShellPath())
	if err != nil {
		return StartResult{}, err
	}
	cmd.Dir = cwd
	cmd.Env = shellEnvironment(ctx, m.workspaces.EffectiveShellEnvironmentPolicy(), m.workspaces.ShellEnvironmentAllow(), m.workspaces.ShellPath(), strict)
	if strict && runtime.GOOS != "windows" {
		cmd.Env = setEnvironmentValue(cmd.Env, "SHELL", cmd.Path)
	}
	roots, err := m.workspaces.EffectiveRoots(workspaceID)
	if err != nil {
		return StartResult{}, err
	}
	cmd, err = wrapShellSandbox(processCtx, cmd, command, cwd, roots, m.workspaces.ShellPath(), m.workspaces.EffectiveShellSandboxPolicy(), m.workspaces.EffectiveShellNetworkPolicy())
	if err != nil {
		return StartResult{}, err
	}
	configureCommandLifecycle(cmd)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return StartResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return StartResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return StartResult{}, err
	}
	id, err := processID()
	if err != nil {
		_ = signalCommandTree(cmd, true)
		return StartResult{}, err
	}
	process := &managedProcess{
		workspace: workspaceID, id: id, command: command, cwd: cwd,
		startedAt: time.Now().UTC().Format(time.RFC3339Nano), cmd: cmd, stdout: &logBuffer{}, stderr: &logBuffer{},
	}
	if m.executions != nil {
		metadata := executionMetadata(ctx)
		process.execution = m.executions.Begin(ExecutionInput{WorkspaceID: workspaceID, Tool: "start_process", Command: command, CWD: cwd, Source: metadata.Source, CallID: metadata.CallID, SessionHash: metadata.SessionHash, ReceivedByInstanceID: metadata.ReceivedByInstanceID, ExecutedByInstanceID: metadata.ExecutedByInstanceID})
	}
	m.mu.Lock()
	m.pruneLocked(time.Now().UTC())
	m.processes[id] = process
	m.order = append(m.order, id)
	m.mu.Unlock()

	var outputWG sync.WaitGroup
	outputWG.Add(2)
	go copyProcessLog(&outputWG, process.stdout, process.execution, "stdout", stdoutPipe)
	go copyProcessLog(&outputWG, process.stderr, process.execution, "stderr", stderrPipe)
	go func() {
		waitErr := cmd.Wait()
		outputWG.Wait()
		process.mu.Lock()
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			code := exitErr.ExitCode()
			process.exitCode = &code
		} else if waitErr == nil {
			code := 0
			process.exitCode = &code
		} else {
			code := -1
			process.exitCode = &code
		}
		if state := cmd.ProcessState; state != nil {
			if text := signalFromState(state.String()); text != "" {
				process.signal = &text
			}
		}
		process.finishedAt = time.Now().UTC()
		exitCode := cloneInt(process.exitCode)
		signal := cloneString(process.signal)
		process.mu.Unlock()
		if process.execution != nil {
			status := ExecutionStatusSuccess
			if signal != nil {
				status = ExecutionStatusCancelled
			} else if exitCode == nil || *exitCode != 0 {
				status = ExecutionStatusFailed
			}
			process.execution.Finish(status, exitCode, false)
		}
		m.mu.Lock()
		m.pruneLocked(time.Now().UTC())
		m.mu.Unlock()
	}()

	executionID := ""
	if process.execution != nil {
		executionID = process.execution.ID()
	}
	return StartResult{ID: id, ExecutionID: executionID, PID: cmd.Process.Pid, Command: command, CWD: cwd, StartedAt: process.startedAt}, nil
}

func (m *ProcessManager) Status(workspaceID, id string) ([]ProcessInfo, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return nil, err
	}
	workspaceID = item.ID
	m.mu.Lock()
	m.pruneLocked(time.Now().UTC())
	items := make([]*managedProcess, 0, len(m.processes))
	for _, item := range m.processes {
		if item.workspace == workspaceID && (id == "" || item.id == id) {
			items = append(items, item)
		}
	}
	m.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].startedAt < items[j].startedAt })
	result := make([]ProcessInfo, 0, len(items))
	for _, item := range items {
		result = append(result, item.info())
	}
	return result, nil
}

func (m *ProcessManager) Output(workspaceID, id string, tailChars int) (OutputResult, error) {
	item, err := m.get(workspaceID, id)
	if err != nil {
		return OutputResult{}, err
	}
	if tailChars <= 0 {
		tailChars = 40_000
	}
	if tailChars > 200_000 {
		return OutputResult{}, errors.New("tail_chars must be <= 200000")
	}
	item.mu.Lock()
	running := item.exitCode == nil
	exitCode := cloneInt(item.exitCode)
	signal := cloneString(item.signal)
	item.mu.Unlock()
	return OutputResult{
		ID: id, Running: running, ExitCode: exitCode, Signal: signal,
		Stdout: item.stdout.tail(tailChars), Stderr: item.stderr.tail(tailChars),
	}, nil
}

func (m *ProcessManager) Stop(workspaceID, id string, force bool) (StopResult, error) {
	item, err := m.get(workspaceID, id)
	if err != nil {
		return StopResult{}, err
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.exitCode != nil {
		return StopResult{ID: id, AlreadyExited: true}, nil
	}
	if item.cmd.Process == nil {
		return StopResult{}, errors.New("process handle is unavailable")
	}
	if err := signalCommandTree(item.cmd, force); err != nil {
		return StopResult{}, err
	}
	return StopResult{ID: id, Force: force}, nil
}

func (m *ProcessManager) Clear(workspaceID string) (int, error) {
	item, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return 0, err
	}
	workspaceID = item.ID
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now().UTC())
	cleared := 0
	for id, item := range m.processes {
		if item.workspace != workspaceID {
			continue
		}
		item.mu.Lock()
		finished := item.exitCode != nil
		item.mu.Unlock()
		if finished {
			delete(m.processes, id)
			cleared++
		}
	}
	m.compactOrderLocked()
	return cleared, nil
}

func (m *ProcessManager) get(workspaceID, id string) (*managedProcess, error) {
	workspace, err := m.workspaces.Get(workspaceID)
	if err != nil {
		return nil, err
	}
	workspaceID = workspace.ID
	m.mu.Lock()
	m.pruneLocked(time.Now().UTC())
	item := m.processes[id]
	m.mu.Unlock()
	if item == nil || item.workspace != workspaceID {
		return nil, fmt.Errorf("unknown process id: %s", id)
	}
	return item, nil
}

func (m *ProcessManager) pruneLocked(now time.Time) {
	if m == nil || len(m.order) == 0 {
		return
	}
	retention := m.retention
	if retention <= 0 {
		retention = finishedProcessRetention
	}
	maxFinished := m.maxFinished
	if maxFinished <= 0 {
		maxFinished = maxFinishedProcesses
	}
	finished := 0
	for _, id := range m.order {
		item := m.processes[id]
		if item == nil {
			continue
		}
		item.mu.Lock()
		isFinished := item.exitCode != nil
		finishedAt := item.finishedAt
		item.mu.Unlock()
		if isFinished && !finishedAt.IsZero() && now.Sub(finishedAt) > retention {
			delete(m.processes, id)
			continue
		}
		if isFinished {
			finished++
		}
	}
	remove := finished - maxFinished
	if remove > 0 {
		for _, id := range m.order {
			if remove == 0 {
				break
			}
			item := m.processes[id]
			if item == nil {
				continue
			}
			item.mu.Lock()
			isFinished := item.exitCode != nil
			item.mu.Unlock()
			if isFinished {
				delete(m.processes, id)
				remove--
			}
		}
	}
	m.compactOrderLocked()
}

func (m *ProcessManager) compactOrderLocked() {
	kept := m.order[:0]
	for _, id := range m.order {
		if m.processes[id] != nil {
			kept = append(kept, id)
		}
	}
	m.order = kept
}

func (p *managedProcess) info() ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	pid := 0
	if p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
	}
	return ProcessInfo{
		ID: p.id, ExecutionID: processExecutionID(p.execution), PID: pid, Command: p.command, CWD: p.cwd, StartedAt: p.startedAt,
		Running: p.exitCode == nil, ExitCode: cloneInt(p.exitCode), Signal: cloneString(p.signal),
	}
}

func copyProcessLog(wg *sync.WaitGroup, target *logBuffer, execution *ExecutionRun, stream string, reader io.Reader) {
	defer wg.Done()
	writers := []io.Writer{target}
	if execution != nil {
		writers = append(writers, execution.Writer(stream))
	}
	_, _ = io.Copy(io.MultiWriter(writers...), reader)
}

func processExecutionID(execution *ExecutionRun) string {
	if execution == nil {
		return ""
	}
	return execution.ID()
}

func (b *logBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	if len(b.data) > maxProcessLogChars {
		b.data = append([]byte(nil), b.data[len(b.data)-maxProcessLogChars:]...)
	}
	return len(data), nil
}

func (b *logBuffer) tail(chars int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if chars >= len(b.data) {
		return string(b.data)
	}
	return string(b.data[len(b.data)-chars:])
}

func processID() (string, error) {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%s", time.Now().UnixMilli(), hex.EncodeToString(buffer)), nil
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func signalFromState(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "interrupt"):
		return "interrupt"
	case strings.Contains(lower, "killed"):
		return "killed"
	default:
		return ""
	}
}
