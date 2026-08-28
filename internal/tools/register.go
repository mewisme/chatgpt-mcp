package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type CommandResult struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
	Stdout           string `json:"stdout"`
	Stderr           string `json:"stderr"`
	ExitCode         int    `json:"exit_code"`
	Success          bool   `json:"success"`
}

type WriteFileResult struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

type ReadTextFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ReadFilesResult struct {
	Files []ReadFile `json:"files"`
	Count int        `json:"count"`
}

type GitStatusResult struct {
	Path   string `json:"path"`
	Output string `json:"output"`
}

func RegisterCore(r *Registry, workspaces *workspace.Manager) {
	r.MustRegister("read_text_file", coreSchema("read_text_file", "Read a text file", `{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"},"path":{"type":"string"}},"required":["workspace_id","working_directory","path"],"additionalProperties":false}`, `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`, RiskRead), handleReadTextFile(workspaces))
	r.MustRegister("read_files", coreSchema("read_files", "Read multiple text files", `{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"},"paths":{"type":"array","items":{"type":"string"},"minItems":1}},"required":["workspace_id","working_directory","paths"],"additionalProperties":false}`, `{"type":"object","properties":{"files":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}},"count":{"type":"integer"}},"required":["files","count"],"additionalProperties":false}`, RiskRead), handleReadFiles(workspaces))
	r.MustRegister("write_file", coreSchema("write_file", "Write a text file", `{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"},"path":{"type":"string"},"content":{"type":"string"}},"required":["workspace_id","working_directory","path","content"],"additionalProperties":false}`, `{"type":"object","properties":{"path":{"type":"string"},"bytes":{"type":"integer"}},"required":["path","bytes"],"additionalProperties":false}`, RiskEdit), handleWriteFile(workspaces))
	r.MustRegister("run_command", coreSchema("run_command", "Run a shell command. Rename/delete operations are denied unless every literal target and cwd can be proven inside the registered workspace.", `{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"},"command":{"type":"string"}},"required":["workspace_id","working_directory","command"],"additionalProperties":false}`, `{"type":"object","properties":{"command":{"type":"string"},"working_directory":{"type":"string"},"stdout":{"type":"string"},"stderr":{"type":"string"},"exit_code":{"type":"integer"},"success":{"type":"boolean"}},"required":["command","working_directory","stdout","stderr","exit_code","success"],"additionalProperties":false}`, RiskCommand), handleRunCommand(workspaces))
	r.MustRegister("git_status", coreSchema("git_status", "Get git status in short format", `{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"}},"required":["workspace_id","working_directory"],"additionalProperties":false}`, `{"type":"object","properties":{"path":{"type":"string"},"output":{"type":"string"}},"required":["path","output"],"additionalProperties":false}`, RiskRead), handleGitStatus(workspaces))
}

func coreSchema(name, description, input, output string, risk Risk) Schema {
	return Schema{
		Name:         name,
		Description:  description,
		InputSchema:  json.RawMessage(input),
		OutputSchema: json.RawMessage(output),
		Annotations:  ToolAnnotations(risk),
	}
}

func handleReadTextFile(workspaces *workspace.Manager) Handler {
	return func(_ context.Context, args map[string]any) (Result, error) {
		_, _, file, err := workspacePath(workspaces, args, "path", true)
		if err != nil {
			return Result{}, err
		}
		content, err := os.ReadFile(file)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(ReadTextFileResult{Path: file, Content: string(content)}), nil
	}
}

func handleReadFiles(workspaces *workspace.Manager) Handler {
	return func(_ context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceContext(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		paths, err := requiredStrings(args, "paths")
		if err != nil {
			return Result{}, err
		}
		files := make([]ReadFile, 0, len(paths))
		for _, value := range paths {
			file, err := workspaces.ResolvePath(item.ID, cwd, value, true)
			if err != nil {
				return Result{}, fmt.Errorf("path %q: %w", value, err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return Result{}, err
			}
			files = append(files, ReadFile{Path: file, Content: string(data)})
		}
		return JSONResult(ReadFilesResult{Files: files, Count: len(files)}), nil
	}
}

func handleWriteFile(workspaces *workspace.Manager) Handler {
	return func(_ context.Context, args map[string]any) (Result, error) {
		_, _, file, err := workspacePath(workspaces, args, "path", false)
		if err != nil {
			return Result{}, err
		}
		content, ok := args["content"].(string)
		if !ok {
			return Result{}, fmt.Errorf("content must be a string")
		}
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			return Result{}, err
		}
		return JSONResult(WriteFileResult{Path: file, Bytes: len([]byte(content))}), nil
	}
}

func handleRunCommand(workspaces *workspace.Manager) Handler {
	return func(ctx context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceContext(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		command, err := requiredString(args, "command")
		if err != nil {
			return Result{}, err
		}
		if err := workspaces.ValidateMutationCommand(item.ID, cwd, command); err != nil {
			return Result{}, err
		}

		cmd := shellCommand(ctx, command)
		cmd.Dir = cwd
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		exitCode := 0
		if runErr != nil {
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) {
				return Result{}, fmt.Errorf("run command: %w", runErr)
			}
			exitCode = exitErr.ExitCode()
		}
		value := CommandResult{Command: command, WorkingDirectory: cwd, Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode, Success: exitCode == 0}
		result := JSONResult(value)
		if exitCode != 0 {
			result.IsError = true
		}
		return result, nil
	}
}

func handleGitStatus(workspaces *workspace.Manager) Handler {
	return func(ctx context.Context, args map[string]any) (Result, error) {
		_, cwd, err := workspaceContext(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		cmd := exec.CommandContext(ctx, "git", "-C", cwd, "status", "--short")
		output, err := cmd.CombinedOutput()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		if err != nil {
			detail := strings.TrimSpace(string(output))
			if detail != "" {
				return Result{}, fmt.Errorf("git status: %w: %s", err, detail)
			}
			return Result{}, fmt.Errorf("git status: %w", err)
		}
		text := string(output)
		if strings.TrimSpace(text) == "" {
			text = "Clean working tree"
		}
		return JSONResult(GitStatusResult{Path: cwd, Output: text}), nil
	}
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key].(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requiredStrings(args map[string]any, key string) ([]string, error) {
	switch values := args[key].(type) {
	case []string:
		if len(values) == 0 {
			return nil, fmt.Errorf("%s must not be empty", key)
		}
		for i, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("%s[%d] must not be empty", key, i)
			}
		}
		return values, nil
	case []any:
		if len(values) == 0 {
			return nil, fmt.Errorf("%s must not be empty", key)
		}
		result := make([]string, len(values))
		for i, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, i)
			}
			result[i] = text
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if shell, err := exec.LookPath("pwsh"); err == nil {
			return exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
		}
		if shell, err := exec.LookPath("powershell"); err == nil {
			return exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
		}
		return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return exec.CommandContext(ctx, shell, "-lc", command)
}
