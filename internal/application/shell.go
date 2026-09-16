package application

import (
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
)

type ShellDiagnostic struct {
	Available   bool
	Provider    shellruntime.Provider
	Configured  string
	Error       string
	Remediation string
}

func LoadShellDiagnostic() ShellDiagnostic {
	diagnostic := ShellDiagnostic{}
	cfg, err := config.Load()
	if err != nil {
		diagnostic.Error = err.Error()
		return diagnostic
	}
	diagnostic.Configured = strings.TrimSpace(cfg.Shell.Executable)
	resolver := shellruntime.DefaultProviderResolver()
	if err := resolver.SetConfiguredExecutable(diagnostic.Configured); err != nil {
		diagnostic.Error = err.Error()
		diagnostic.Remediation = shellRemediation()
		return diagnostic
	}
	provider, err := resolver.Resolve()
	if err != nil {
		diagnostic.Error = err.Error()
		diagnostic.Remediation = shellRemediation()
		return diagnostic
	}
	diagnostic.Available = true
	diagnostic.Provider = provider
	return diagnostic
}

func shellRemediation() string {
	if runtime.GOOS == "windows" {
		return "cgm plugin install bash"
	}
	return "Install Bash and ensure it is available on PATH"
}
