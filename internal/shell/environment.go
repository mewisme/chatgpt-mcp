package shell

import (
	"context"
	"os"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type environmentValue struct {
	name  string
	value string
}

var shellEnvironmentDangerous = map[string]bool{
	"BASH_ENV": true, "ENV": true, "PROMPT_COMMAND": true, "NODE_OPTIONS": true, "PYTHONPATH": true, "PYTHONSTARTUP": true,
	"RUBYOPT": true, "PERL5OPT": true, "JAVA_TOOL_OPTIONS": true, "_JAVA_OPTIONS": true, "LD_PRELOAD": true, "DYLD_INSERT_LIBRARIES": true,
	"GIT_EXTERNAL_DIFF": true, "GIT_SSH": true, "GIT_SSH_COMMAND": true, "GIT_ASKPASS": true, "SSH_ASKPASS": true, "SUDO_ASKPASS": true,
	"SSH_AUTH_SOCK": true, "GPG_AGENT_INFO": true, "SSLKEYLOGFILE": true, "KUBECONFIG": true, "DOCKER_CERT_PATH": true,
	"GOOGLE_APPLICATION_CREDENTIALS": true, "AWS_SHARED_CREDENTIALS_FILE": true, "AWS_CONFIG_FILE": true, "AZURE_CONFIG_DIR": true, "NETRC": true,
	"DATABASE_URL": true, "REDIS_URL": true, "MONGODB_URI": true, "POSTGRES_URL": true, "POSTGRESQL_URL": true,
}

var shellEnvironmentMinimal = map[string]bool{
	"PATH": true, "HOME": true, "USERPROFILE": true, "USER": true, "USERNAME": true, "LOGNAME": true, "SHELL": true, "COMSPEC": true,
	"PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "SYSTEMDRIVE": true, "HOMEDRIVE": true, "HOMEPATH": true,
	"TEMP": true, "TMP": true, "TMPDIR": true, "LANG": true, "LANGUAGE": true, "TERM": true, "COLORTERM": true, "TZ": true,
	"GOROOT": true, "GOPATH": true, "GOMODCACHE": true, "GOCACHE": true, "GOENV": true, "GOTOOLCHAIN": true, "GOFLAGS": true, "GOOS": true, "GOARCH": true,
	"CGO_ENABLED": true, "CC": true, "CXX": true, "CARGO_HOME": true, "RUSTUP_HOME": true, "JAVA_HOME": true, "ANDROID_HOME": true,
	"ANDROID_SDK_ROOT": true, "DOTNET_ROOT": true, "PNPM_HOME": true, "NPM_CONFIG_CACHE": true, "COREPACK_HOME": true, "VIRTUAL_ENV": true,
	"CONDA_PREFIX": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "NODE_EXTRA_CA_CERTS": true, "REQUESTS_CA_BUNDLE": true, "CURL_CA_BUNDLE": true,
}

func shellEnvironment(ctx context.Context, policy workspace.ShellEnvironmentPolicy, allow []string) []string {
	parent := parentEnvironment()
	allowed := environmentNameSet(allow)
	values := map[string]environmentValue{}
	for key, entry := range parent {
		if protectedShellEnvironment(key) {
			continue
		}
		include := false
		switch policy {
		case workspace.ShellEnvironmentInherit:
			include = true
		case workspace.ShellEnvironmentFiltered:
			include = !sensitiveShellEnvironment(key)
		case workspace.ShellEnvironmentMinimal:
			include = minimalShellEnvironment(key)
		default:
			include = false
		}
		if allowed[key] {
			include = true
		}
		if include {
			values[key] = entry
		}
	}
	setShellEnvironment(values, "CI", "true")
	setShellEnvironment(values, "PAGER", "cat")
	setShellEnvironment(values, "GIT_PAGER", "cat")
	setShellEnvironment(values, "NO_COLOR", "1")
	setShellEnvironment(values, "npm_config_yes", "true")
	setShellEnvironment(values, controlplane.ToolContextEnv, "1")
	setShellEnvironment(values, configformat.EnvConfigDir, configformat.RootPath())
	if granted, ok := controlguard.ApprovalFromContext(ctx); ok {
		setShellEnvironment(values, controlplane.ControlApprovalEnv, granted.Capability)
	} else {
		delete(values, strings.ToUpper(controlplane.ControlApprovalEnv))
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		entry := values[key]
		out = append(out, entry.name+"="+entry.value)
	}
	return out
}

func parentEnvironment() map[string]environmentValue {
	values := map[string]environmentValue{}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		values[strings.ToUpper(name)] = environmentValue{name: name, value: value}
	}
	return values
}

func environmentNameSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[strings.ToUpper(value)] = true
		}
	}
	return result
}

func setShellEnvironment(values map[string]environmentValue, name, value string) {
	values[strings.ToUpper(name)] = environmentValue{name: name, value: value}
}

func protectedShellEnvironment(name string) bool {
	switch strings.ToUpper(name) {
	case strings.ToUpper(controlplane.ToolContextEnv), strings.ToUpper(controlplane.ControlApprovalEnv), strings.ToUpper(configformat.EnvConfigDir):
		return true
	default:
		return false
	}
}

func minimalShellEnvironment(name string) bool {
	upper := strings.ToUpper(name)
	return shellEnvironmentMinimal[upper] || strings.HasPrefix(upper, "LC_")
}

func sensitiveShellEnvironment(name string) bool {
	upper := strings.ToUpper(name)
	if shellEnvironmentDangerous[upper] || strings.HasPrefix(upper, "GIT_CONFIG_") {
		return true
	}
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "API_KEY", "PRIVATE_KEY", "ACCESS_KEY", "SECRET_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return strings.HasSuffix(upper, "_DSN")
}
