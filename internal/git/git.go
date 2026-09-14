package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxOutputBytes = 400_000

type boundedBuffer struct {
	data      []byte
	truncated bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.data = append(b.data, data...)
	if len(b.data) > maxOutputBytes {
		b.truncated = true
		b.data = append([]byte(nil), b.data[len(b.data)-maxOutputBytes:]...)
	}
	return len(data), nil
}

func (b *boundedBuffer) String() string {
	text := string(b.data)
	if b.truncated {
		return "[output truncated to last 400000 bytes]\n" + text
	}
	return text
}

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func Run(ctx context.Context, cwd string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	cmd.Env = environment()
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, ctxErr
	}
	result := Result{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String()), ExitCode: 0}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return Result{}, errors.New("git not found")
	}
	return Result{}, fmt.Errorf("run git: %w", err)
}

func OrThrow(ctx context.Context, cwd string, args ...string) (Result, error) {
	result, err := Run(ctx, cwd, args...)
	if err != nil {
		return Result{}, err
	}
	if result.ExitCode != 0 {
		detail := result.Stderr
		if detail == "" {
			detail = result.Stdout
		}
		if detail == "" {
			detail = fmt.Sprintf("git exited with code %d", result.ExitCode)
		}
		return Result{}, errors.New(detail)
	}
	return result, nil
}

func environment() []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		if index := strings.IndexByte(entry, '='); index >= 0 {
			values[entry[:index]] = entry[index+1:]
		}
	}
	values["PAGER"] = "cat"
	values["GIT_PAGER"] = "cat"
	values["NO_COLOR"] = "1"
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}
