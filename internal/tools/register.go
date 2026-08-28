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

func RegisterCore(r *Registry) {
	r.MustRegister("read_text_file", coreSchema("read_text_file", "Read a text file", `{"type":"object","properties":{"working_directory":{"type":"string","description":"Base working directory for relative paths"},"path":{"type":"string","description":"File path, absolute or relative to working_directory"}},"required":["working_directory","path"],"additionalProperties":false}`, `{"type":"string"}`, true), handleReadTextFile)
	r.MustRegister("read_files", coreSchema("read_files", "Read multiple text files", `{"type":"object","properties":{"working_directory":{"type":"string","description":"Base working directory for relative paths"},"paths":{"type":"array","items":{"type":"string"},"minItems":1,"description":"File paths, absolute or relative to working_directory"}},"required":["working_directory","paths"],"additionalProperties":false}`, `{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}}`, true), handleReadFiles)
	r.MustRegister("write_file", coreSchema("write_file", "Write a text file", `{"type":"object","properties":{"working_directory":{"type":"string","description":"Base working directory for relative paths"},"path":{"type":"string","description":"File path, absolute or relative to working_directory"},"content":{"type":"string","description":"Complete text content to write"}},"required":["working_directory","path","content"],"additionalProperties":false}`, `{"type":"object","properties":{"path":{"type":"string"},"bytes":{"type":"integer"}},"required":["path","bytes"],"additionalProperties":false}`, false), handleWriteFile)
	r.MustRegister("run_command", coreSchema("run_command", "Run a shell command", `{"type":"object","properties":{"working_directory":{"type":"string","description":"Directory in which the command is executed"},"command":{"type":"string","description":"Shell command to execute"}},"required":["working_directory","command"],"additionalProperties":false}`, `{"type":"object","properties":{"command":{"type":"string"},"working_directory":{"type":"string"},"stdout":{"type":"string"},"stderr":{"type":"string"},"exit_code":{"type":"integer"},"success":{"type":"boolean"}},"required":["command","working_directory","stdout","stderr","exit_code","success"],"additionalProperties":false}`, false), handleRunCommand)
	r.MustRegister("git_status", coreSchema("git_status", "Get git status in short format", `{"type":"object","properties":{"working_directory":{"type":"string","description":"Git repository or a directory inside it"}},"required":["working_directory"],"additionalProperties":false}`, `{"type":"string"}`, true), handleGitStatus)
}

func coreSchema(name, description, input, output string, readOnly bool) Schema {
	return Schema{Name: name, Description: description, InputSchema: json.RawMessage(input), OutputSchema: json.RawMessage(output), Annotations: map[string]any{"readOnly": readOnly}}
}

func handleReadTextFile(_ context.Context, args map[string]any) (Result, error) {
	workdir, err := requiredWorkingDirectory(args)
	if err != nil {
		return Result{}, err
	}
	file, err := requiredString(args, "path")
	if err != nil {
		return Result{}, err
	}
	content, err := (FileService{}).ReadText(workdir, file)
	if err != nil {
		return Result{}, err
	}
	return TextResult(content), nil
}

func handleReadFiles(_ context.Context, args map[string]any) (Result, error) {
	workdir, err := requiredWorkingDirectory(args)
	if err != nil {
		return Result{}, err
	}
	paths, err := requiredStrings(args, "paths")
	if err != nil {
		return Result{}, err
	}
	files, err := (ReadFilesService{}).Read(Context{WorkingDirectory: workdir}, paths)
	if err != nil {
		return Result{}, err
	}
	return JSONResult(files), nil
}

func handleWriteFile(_ context.Context, args map[string]any) (Result, error) {
	workdir, err := requiredWorkingDirectory(args)
	if err != nil {
		return Result{}, err
	}
	file, err := requiredString(args, "path")
	if err != nil {
		return Result{}, err
	}
	content, ok := args["content"].(string)
	if !ok {
		return Result{}, fmt.Errorf("content must be a string")
	}
	if err := (FileService{}).WriteText(workdir, file, content); err != nil {
		return Result{}, err
	}
	return JSONResult(WriteFileResult{Path: resolve(workdir, file), Bytes: len([]byte(content))}), nil
}

func handleRunCommand(ctx context.Context, args map[string]any) (Result, error) {
	workdir, err := requiredWorkingDirectory(args)
	if err != nil {
		return Result{}, err
	}
	command, err := requiredString(args, "command")
	if err != nil {
		return Result{}, err
	}
	cmd := shellCommand(ctx, command)
	cmd.Dir = workdir
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
	value := CommandResult{Command: command, WorkingDirectory: workdir, Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode, Success: exitCode == 0}
	result := JSONResult(value)
	if exitCode != 0 {
		result.IsError = true
	}
	return result, nil
}

func handleGitStatus(ctx context.Context, args map[string]any) (Result, error) {
	workdir, err := requiredWorkingDirectory(args)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", workdir, "status", "--short")
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
	return TextResult(string(output)), nil
}

func requiredWorkingDirectory(args map[string]any) (string, error) {
	workdir, err := requiredString(args, "working_directory")
	if err != nil {
		return "", err
	}
	workdir, err = filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve working_directory: %w", err)
	}
	info, err := os.Stat(workdir)
	if err != nil {
		return "", fmt.Errorf("working_directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working_directory is not a directory: %s", workdir)
	}
	return workdir, nil
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
