package notification

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type runSpec struct {
	Name  string
	Args  []string
	Env   []string
	Stdin string
}

type host struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	runFn    func(context.Context, runSpec) error
	outputFn func(context.Context, runSpec) ([]byte, error)
	readFile func(string) ([]byte, error)
}

func defaultHost() host {
	return host{goos: runtime.GOOS}
}

func (h host) env(key string) string {
	if h.getenv != nil {
		return h.getenv(key)
	}
	return os.Getenv(key)
}

func (h host) lookup(name string) (string, error) {
	if h.lookPath != nil {
		return h.lookPath(name)
	}
	return exec.LookPath(name)
}

func (h host) has(name string) bool {
	_, err := h.lookup(name)
	return err == nil
}

func (h host) read(path string) ([]byte, error) {
	if h.readFile != nil {
		return h.readFile(path)
	}
	return os.ReadFile(path)
}

func (h host) run(ctx context.Context, spec runSpec) error {
	if h.runFn != nil {
		return h.runFn(ctx, spec)
	}
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(out))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func (h host) output(ctx context.Context, spec runSpec) ([]byte, error) {
	if h.outputFn != nil {
		return h.outputFn(ctx, spec)
	}
	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	return cmd.CombinedOutput()
}

func (h host) runningOnWSL() bool {
	if h.goos == "windows" {
		return false
	}
	if strings.TrimSpace(h.env("WSL_DISTRO_NAME")) != "" || strings.TrimSpace(h.env("WSL_INTEROP")) != "" {
		return true
	}
	data, err := h.read("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	value := strings.ToLower(string(data))
	return strings.Contains(value, "microsoft") || strings.Contains(value, "wsl")
}
