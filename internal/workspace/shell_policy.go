package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/commandpattern"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
)

const maxNestedShellDepth = 4

type ShellApprovalPolicy string
type ShellEnvironmentPolicy string
type ShellSandboxPolicy string
type ShellNetworkPolicy string

const (
	ShellApprovalAllow       ShellApprovalPolicy    = "allow"
	ShellApprovalBalanced    ShellApprovalPolicy    = "balanced"
	ShellApprovalStrict      ShellApprovalPolicy    = "strict"
	ShellApprovalDeny        ShellApprovalPolicy    = "deny"
	ShellEnvironmentAuto     ShellEnvironmentPolicy = "auto"
	ShellEnvironmentInherit  ShellEnvironmentPolicy = "inherit"
	ShellEnvironmentFiltered ShellEnvironmentPolicy = "filtered"
	ShellEnvironmentMinimal  ShellEnvironmentPolicy = "minimal"
	ShellSandboxAuto         ShellSandboxPolicy     = "auto"
	ShellSandboxOff          ShellSandboxPolicy     = "off"
	ShellSandboxRequired     ShellSandboxPolicy     = "required"
	ShellNetworkAuto         ShellNetworkPolicy     = "auto"
	ShellNetworkInherit      ShellNetworkPolicy     = "inherit"
	ShellNetworkDeny         ShellNetworkPolicy     = "deny"
)

var (
	absolutePathLiteral = regexp.MustCompile(`(?i)(?:[a-z]:[\\/]|/)[^\"'()\s,;]+`)
	inlineMutationAPI   = regexp.MustCompile(`(?i)(?:\bopen\s*\(|\b(?:write_text|write_bytes|writefile|writefilesync|appendfile|appendfilesync|createwritestream|unlink|unlinksync|rename|renamesync|copyfile|copyfilesync|mkdir|mkdirsync|rmdir|rmdirsync|truncate|remove|replace|rmtree|move)\s*\(|\bos\.system\s*\(|\bsubprocess\.|\bchild_process\b|\bexecsync\s*\(|\bspawnsync\s*\()`)
	toolContextMutation = regexp.MustCompile(`(?i)(?:\bunset\s+CHATGPT_MCP_TOOL_CONTEXT\b|\benv\b[^\r\n;&|]*(?:-u\s+CHATGPT_MCP_TOOL_CONTEXT\b|--unset(?:=|\s+)CHATGPT_MCP_TOOL_CONTEXT\b)|\b(?:set|setx)\s+CHATGPT_MCP_TOOL_CONTEXT\s*=|\bRemove-Item\s+(?:Env:|env:\\)CHATGPT_MCP_TOOL_CONTEXT\b|\bos\.environ\s*\.\s*(?:pop|__delitem__)\s*\(\s*["']CHATGPT_MCP_TOOL_CONTEXT["']|\bdelete\s+process\.env\.CHATGPT_MCP_TOOL_CONTEXT\b)`)
	windowsEnvReference = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)
	writeCommands       = map[string]bool{
		"cp": true, "copy": true, "xcopy": true, "robocopy": true, "install": true, "touch": true, "mkdir": true, "md": true,
		"tee": true, "truncate": true, "ln": true, "link": true, "mkfifo": true,
		"new-item": true, "set-content": true, "add-content": true, "out-file": true, "copy-item": true,
	}
	pathMutationCommands = map[string]bool{
		"chmod": true, "chown": true, "chgrp": true, "sed": true, "perl": true, "dd": true, "rsync": true, "curl": true, "wget": true,
	}
	writeMinimumOperands = map[string]int{
		"cp": 2, "copy": 2, "xcopy": 2, "robocopy": 2, "install": 2, "ln": 2, "link": 2, "copy-item": 2,
	}
	powerShellPathParameters = map[string]bool{
		"-path": true, "-literalpath": true, "-destination": true, "-filepath": true,
	}
)

func (m *Manager) ValidateShellCommand(id, baseDirectory, command string) error {
	return m.ValidateShellCommandContext(context.Background(), id, baseDirectory, command)
}

func NormalizeShellApprovalPolicy(value string) (ShellApprovalPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ShellApprovalBalanced):
		return ShellApprovalBalanced, true
	case string(ShellApprovalAllow):
		return ShellApprovalAllow, true
	case string(ShellApprovalStrict):
		return ShellApprovalStrict, true
	case string(ShellApprovalDeny):
		return ShellApprovalDeny, true
	default:
		return "", false
	}
}

func (m *Manager) SetShellApprovalCommands(allow, deny []string) error {
	allowPatterns, err := commandpattern.Compile(allow)
	if err != nil {
		return err
	}
	denyPatterns, err := commandpattern.Compile(deny)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.shellApprovalAllow = allowPatterns
	m.shellApprovalDeny = denyPatterns
	m.mu.Unlock()
	return nil
}

func (m *Manager) ShellApprovalCommands() (allow, deny []string) {
	if m == nil {
		return []string{}, []string{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	allow = make([]string, 0, len(m.shellApprovalAllow))
	deny = make([]string, 0, len(m.shellApprovalDeny))
	for _, pattern := range m.shellApprovalAllow {
		allow = append(allow, pattern.Raw())
	}
	for _, pattern := range m.shellApprovalDeny {
		deny = append(deny, pattern.Raw())
	}
	return allow, deny
}

func (m *Manager) SetShellApprovalPolicy(value ShellApprovalPolicy) error {
	policy, ok := NormalizeShellApprovalPolicy(string(value))
	if !ok {
		return fmt.Errorf("unsupported shell approval policy: %q", value)
	}
	m.mu.Lock()
	m.shellPolicy = policy
	m.mu.Unlock()
	return nil
}

func (m *Manager) ShellApprovalPolicy() ShellApprovalPolicy {
	if m == nil {
		return ShellApprovalBalanced
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shellPolicy == "" {
		return ShellApprovalBalanced
	}
	return m.shellPolicy
}

func NormalizeShellEnvironmentPolicy(value string) (ShellEnvironmentPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ShellEnvironmentAuto):
		return ShellEnvironmentAuto, true
	case string(ShellEnvironmentInherit):
		return ShellEnvironmentInherit, true
	case string(ShellEnvironmentFiltered):
		return ShellEnvironmentFiltered, true
	case string(ShellEnvironmentMinimal):
		return ShellEnvironmentMinimal, true
	default:
		return "", false
	}
}

func (m *Manager) SetShellEnvironmentPolicy(value ShellEnvironmentPolicy) error {
	policy, ok := NormalizeShellEnvironmentPolicy(string(value))
	if !ok {
		return fmt.Errorf("unsupported shell environment policy: %q", value)
	}
	m.mu.Lock()
	m.shellEnvPolicy = policy
	m.mu.Unlock()
	return nil
}

func (m *Manager) ShellEnvironmentPolicy() ShellEnvironmentPolicy {
	if m == nil {
		return ShellEnvironmentAuto
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shellEnvPolicy == "" {
		return ShellEnvironmentAuto
	}
	return m.shellEnvPolicy
}

func (m *Manager) EffectiveShellEnvironmentPolicy() ShellEnvironmentPolicy {
	policy := m.ShellEnvironmentPolicy()
	if policy != ShellEnvironmentAuto {
		return policy
	}
	if policy := m.ShellApprovalPolicy(); policy == ShellApprovalStrict || policy == ShellApprovalDeny {
		return ShellEnvironmentMinimal
	}
	return ShellEnvironmentInherit
}

func NormalizeShellSandboxPolicy(value string) (ShellSandboxPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ShellSandboxAuto):
		return ShellSandboxAuto, true
	case string(ShellSandboxOff):
		return ShellSandboxOff, true
	case string(ShellSandboxRequired):
		return ShellSandboxRequired, true
	default:
		return "", false
	}
}

func (m *Manager) SetShellSandboxPolicy(value ShellSandboxPolicy) error {
	policy, ok := NormalizeShellSandboxPolicy(string(value))
	if !ok {
		return fmt.Errorf("unsupported shell sandbox policy: %q", value)
	}
	m.mu.Lock()
	m.shellSandboxPolicy = policy
	m.mu.Unlock()
	return nil
}

func (m *Manager) ShellSandboxPolicy() ShellSandboxPolicy {
	if m == nil {
		return ShellSandboxAuto
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shellSandboxPolicy == "" {
		return ShellSandboxAuto
	}
	return m.shellSandboxPolicy
}

func (m *Manager) EffectiveShellSandboxPolicy() ShellSandboxPolicy {
	policy := m.ShellSandboxPolicy()
	if policy != ShellSandboxAuto {
		return policy
	}
	if policy := m.ShellApprovalPolicy(); policy == ShellApprovalStrict || policy == ShellApprovalDeny {
		return ShellSandboxAuto
	}
	return ShellSandboxOff
}

func NormalizeShellNetworkPolicy(value string) (ShellNetworkPolicy, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ShellNetworkAuto):
		return ShellNetworkAuto, true
	case string(ShellNetworkInherit):
		return ShellNetworkInherit, true
	case string(ShellNetworkDeny):
		return ShellNetworkDeny, true
	default:
		return "", false
	}
}

func (m *Manager) SetShellNetworkPolicy(value ShellNetworkPolicy) error {
	policy, ok := NormalizeShellNetworkPolicy(string(value))
	if !ok {
		return fmt.Errorf("unsupported shell network policy: %q", value)
	}
	m.mu.Lock()
	m.shellNetworkPolicy = policy
	m.mu.Unlock()
	return nil
}

func (m *Manager) ShellNetworkPolicy() ShellNetworkPolicy {
	if m == nil {
		return ShellNetworkAuto
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.shellNetworkPolicy == "" {
		return ShellNetworkAuto
	}
	return m.shellNetworkPolicy
}

func (m *Manager) EffectiveShellNetworkPolicy() ShellNetworkPolicy {
	policy := m.ShellNetworkPolicy()
	if policy != ShellNetworkAuto {
		return policy
	}
	if policy := m.ShellApprovalPolicy(); policy == ShellApprovalStrict || policy == ShellApprovalDeny {
		return ShellNetworkAuto
	}
	return ShellNetworkInherit
}

func (m *Manager) ValidateShellCommandContext(ctx context.Context, id, baseDirectory, command string) error {
	_, cwd, err := m.ResolveDirectory(id, baseDirectory)
	if err != nil {
		return err
	}
	if toolContextMutation.MatchString(command) {
		return controlguard.New(controlguard.CodeContextTamper, "MCP tool execution context cannot be cleared from shell commands", false, nil)
	}
	if err := m.validateProtectedShellAccess(cwd, command, 0); err != nil {
		return controlguard.New(controlguard.CodeProtectedState, err.Error(), false, nil)
	}
	if reason, denied := unboundedRemoteSessionReason(command); denied {
		return controlguard.New(controlguard.CodeExternalMutation, "unbounded remote session denied from MCP shell: "+reason, false, nil)
	}
	networkPolicy := m.EffectiveShellNetworkPolicy()
	if networkPolicy == ShellNetworkDeny {
		if reason, ok := externalMutationReason(command); ok {
			return controlguard.New(controlguard.CodeExternalMutation, "external mutation denied by shell network policy: "+reason, false, nil)
		}
		if reason, ok := externalAccessReason(command); ok {
			return controlguard.New(controlguard.CodeExternalAccess, "external access denied by shell network policy: "+reason, false, nil)
		}
	}
	if isControlPlaneMutation(command, 0) {
		invocation, approvable := DirectControlPlaneInvocation(command)
		if approvable && invocation != nil {
			if granted, ok := controlguard.ApprovalFromContext(ctx); ok && controlguard.SameInvocation(granted.Invocation, *invocation) {
				return nil
			}
		}
		return controlguard.New(controlguard.CodeControlPlaneMutation, "control-plane mutation denied from MCP shell: chatgpt-mcp configuration and permissions cannot be changed through shell tools", approvable, invocation)
	}
	mutation := m.IsMutationCommand(command)
	if mutation {
		if err := m.ValidateMutationCommand(id, baseDirectory, command); err != nil {
			return err
		}
	}
	denied, allowed := m.shellApprovalRuleDecision(command)
	if denied {
		code, category, reason := shellApprovalRequirement(command)
		if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == code {
			return nil
		}
		return controlguard.New(code, category+" shell execution requires local approval by explicit deny rule: "+reason, true, &controlguard.Invocation{Command: strings.TrimSpace(command)})
	}
	if allowed || m.ShellApprovalPolicy() == ShellApprovalAllow {
		return nil
	}
	if m.ShellApprovalPolicy() == ShellApprovalDeny {
		code, category, reason := shellApprovalRequirement(command)
		if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == code {
			return nil
		}
		return controlguard.New(code, category+" shell execution requires local approval by deny policy: "+reason, true, &controlguard.Invocation{Command: strings.TrimSpace(command)})
	}
	if code, category, reason, guarded := shellApprovalRisk(command); guarded {
		if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == code {
			return nil
		}
		invocation := &controlguard.Invocation{Command: strings.TrimSpace(command)}
		return controlguard.New(code, category+" shell mutation requires local approval: "+reason, true, invocation)
	}
	if networkPolicy == ShellNetworkAuto {
		if reason, ok := externalAccessReason(command); ok {
			if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == controlguard.CodeExternalAccess {
				return nil
			}
			return controlguard.New(controlguard.CodeExternalAccess, "external shell access requires local approval: "+reason, true, &controlguard.Invocation{Command: strings.TrimSpace(command)})
		}
	}
	if m.ShellApprovalPolicy() == ShellApprovalStrict && !m.staticallyReadOnlyShellCommand(id, cwd, command) {
		if grant, ok := controlguard.GrantFromContext(ctx); ok && grant.Code == controlguard.CodeShellExecution {
			return nil
		}
		return controlguard.New(controlguard.CodeShellExecution, "strict shell policy requires local approval for execution that is not a workspace-confined static read", true, &controlguard.Invocation{Command: strings.TrimSpace(command)})
	}
	return nil
}

func shellApprovalRequirement(command string) (controlguard.Code, string, string) {
	if code, category, reason, guarded := shellApprovalRisk(command); guarded {
		return code, category, reason
	}
	if reason, ok := externalAccessReason(command); ok {
		return controlguard.CodeExternalAccess, "external", reason
	}
	return controlguard.CodeShellExecution, "shell", "command matched the configured approval policy"
}

func (m *Manager) shellApprovalRuleDecision(command string) (denied, allowed bool) {
	m.mu.RLock()
	allowPatterns := append([]commandpattern.Pattern(nil), m.shellApprovalAllow...)
	denyPatterns := append([]commandpattern.Pattern(nil), m.shellApprovalDeny...)
	m.mu.RUnlock()
	if len(allowPatterns) == 0 && len(denyPatterns) == 0 {
		return false, false
	}
	invocations, err := shellCommandInvocations(command, 0)
	if err != nil || len(invocations) == 0 {
		return false, false
	}
	allAllowed := len(allowPatterns) > 0
	for _, invocation := range invocations {
		if commandpattern.MatchAny(denyPatterns, invocation) {
			return true, false
		}
		if !commandpattern.MatchAny(allowPatterns, invocation) {
			allAllowed = false
		}
	}
	return false, allAllowed
}

func shellCommandInvocations(command string, depth int) ([][]string, error) {
	if depth >= maxNestedShellDepth {
		return nil, errors.New("nested shell depth exceeded")
	}
	segments, err := splitShellSegments(command)
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0, len(segments))
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil {
			return nil, err
		}
		if len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if name == "" {
			continue
		}
		invocation := append([]string{name}, args...)
		result = append(result, invocation)
		if inner, ok := nestedShellCommand(name, args); ok {
			nested, err := shellCommandInvocations(inner, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, nested...)
		}
	}
	return result, nil
}

func (m *Manager) staticallyReadOnlyShellCommand(id, cwd, command string) bool {
	if targets, err := outputRedirectionTargets(command); err != nil || len(targets) > 0 {
		return false
	}
	if hasDynamicShellExpansion(command) {
		return false
	}
	segments, err := splitShellSegments(command)
	if err != nil || len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			return false
		}
		name, args := commandName(tokens)
		if _, nested := nestedShellCommand(name, args); nested {
			return false
		}
		if !m.staticallyReadOnlyInvocation(id, cwd, name, args) {
			return false
		}
	}
	return true
}

func (m *Manager) staticallyReadOnlyInvocation(id, cwd, name string, args []string) bool {
	switch name {
	case "pwd", "printf", "echo", "basename", "dirname", "uname", "whoami", "id", "date", "which", "where", "whereis":
		return true
	case "ls", "dir", "cat", "stat", "head", "tail", "wc", "realpath", "readlink":
		paths, ok := staticReadPaths(name, args)
		return ok && m.staticReadPathsAllowed(id, cwd, paths)
	}
	return false
}

func hasDynamicShellExpansion(command string) bool {
	if strings.ContainsAny(command, "$`") || strings.Contains(command, "<(") || strings.Contains(command, ">(") || strings.Contains(command, "<") {
		return true
	}
	return windowsEnvReference.MatchString(command)
}

func staticReadPaths(name string, args []string) ([]string, bool) {
	valueOptions := map[string]bool{}
	switch name {
	case "head", "tail":
		valueOptions = map[string]bool{"-n": true, "--lines": true, "-c": true, "--bytes": true, "--sleep-interval": true, "--pid": true, "-s": true}
	case "stat":
		valueOptions = map[string]bool{"-c": true, "--format": true, "--printf": true}
	case "realpath":
		for index, arg := range args {
			lower := strings.ToLower(arg)
			if lower == "--relative-to" || lower == "--relative-base" || strings.HasPrefix(lower, "--relative-to=") || strings.HasPrefix(lower, "--relative-base=") {
				if !strings.Contains(arg, "=") && index+1 >= len(args) {
					return nil, false
				}
				return nil, false
			}
		}
	}
	paths, err := commandPositionals(args, valueOptions)
	if err != nil {
		return nil, false
	}
	return paths, true
}

func (m *Manager) staticReadPathsAllowed(id, cwd string, paths []string) bool {
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" || value == "-" || isNullDevice(value) {
			continue
		}
		if unsafeShellPath(value) {
			return false
		}
		if _, err := m.ResolvePath(id, cwd, value, false); err != nil {
			return false
		}
	}
	return true
}

func shellApprovalRisk(command string) (controlguard.Code, string, string, bool) {
	if reason, ok := externalMutationReason(command); ok {
		return controlguard.CodeExternalMutation, "external", reason, true
	}
	if reason, ok := hostMutationReason(command); ok {
		return controlguard.CodeHostMutation, "host", reason, true
	}
	if reason, ok := destructiveMutationReason(command); ok {
		return controlguard.CodeDestructiveMutation, "destructive", reason, true
	}
	return "", "", "", false
}

func unboundedRemoteSessionReason(command string) (string, bool) {
	return unboundedRemoteSessionReasonDepth(command, 0)
}

func unboundedRemoteSessionReasonDepth(command string, depth int) (string, bool) {
	if depth >= maxNestedShellDepth {
		return "nested remote session depth exceeded", true
	}
	segments, err := splitShellSegments(command)
	if err != nil {
		return "", false
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			continue
		}
		name, _ := commandName(tokens)
		switch name {
		case "sftp", "ftp", "telnet":
			return name + " interactive session cannot be statically bounded", true
		}
		name, args := commandName(tokens)
		if inner, ok := nestedShellCommand(name, args); ok {
			if reason, denied := unboundedRemoteSessionReasonDepth(inner, depth+1); denied {
				return reason, true
			}
		}
	}
	return "", false
}

func DirectControlPlaneInvocation(command string) (*controlguard.Invocation, bool) {
	segments, err := splitShellSegments(command)
	if err != nil || len(segments) != 1 || strings.TrimSpace(command) != strings.TrimSpace(segments[0]) {
		return nil, false
	}
	tokens, err := shellWords(segments[0])
	if err != nil || len(tokens) == 0 || !isChatGPTMCPBinary(tokens[0]) {
		return nil, false
	}
	args := append([]string(nil), tokens[1:]...)
	if !controlplane.ApprovalEligibleArgs(args) {
		return nil, false
	}
	return &controlguard.Invocation{Program: filepath.Base(tokens[0]), Args: args, Command: strings.TrimSpace(command)}, true
}

func ShellCommandSummary(command string) string {
	segments, err := splitShellSegments(command)
	if err != nil || len(segments) == 0 {
		return "Run shell command"
	}
	if len(segments) > 1 {
		return fmt.Sprintf("Run %d shell commands", len(segments))
	}
	tokens, err := shellWords(segments[0])
	if err != nil || len(tokens) == 0 {
		return "Run shell command"
	}
	name, args := commandName(tokens)
	switch name {
	case "cgm", "cmcp", "chatgpt-mcp":
		switch firstCommandArg(args) {
		case "update":
			return "Update ChatGPT MCP"
		case "install":
			return "Install ChatGPT MCP"
		case "uninstall":
			return "Uninstall ChatGPT MCP"
		case "config":
			return "Modify ChatGPT MCP configuration"
		case "workspace":
			return "Modify ChatGPT MCP workspace state"
		}
	case "rm", "rmdir", "unlink", "del", "erase", "remove-item", "shred":
		return "Delete files"
	case "truncate", "clear-content":
		return "Clear file contents"
	case "mv", "move", "ren", "rename", "move-item", "rename-item":
		return "Move or rename files"
	case "git":
		if subcommand, rest, ok := gitCommand(args); ok {
			switch subcommand {
			case "push":
				return "Push Git commits"
			case "rm", "clean":
				return "Delete Git files"
			case "reset":
				if containsAnyFold(rest, "--hard", "--merge", "--keep") {
					return "Reset Git working tree"
				}
			case "restore", "checkout", "switch":
				return "Modify Git working tree"
			case "branch":
				if containsAnyFold(rest, "-D", "--delete") {
					return "Delete Git branch"
				}
			case "tag":
				if containsAnyFold(rest, "-d", "--delete") {
					return "Delete Git tag"
				}
			}
		}
	case "systemctl":
		return "Modify system service"
	case "service":
		return "Modify system service"
	case "kill", "pkill", "killall", "taskkill", "stop-process":
		return "Terminate process"
	case "shutdown", "reboot", "poweroff", "halt", "restart-computer", "stop-computer":
		return "Change host power state"
	case "docker", "podman":
		if firstCommandArg(args, "--context", "-h", "--host", "--config", "--log-level") == "push" {
			return "Push container image"
		}
		return "Modify container runtime"
	case "kubectl":
		return "Modify Kubernetes cluster"
	case "helm":
		return "Modify Helm release"
	case "terraform", "tofu":
		return "Modify infrastructure"
	case "npm", "pnpm", "yarn", "bun", "cargo", "twine":
		if registryPublishMutation(name, args) {
			return "Publish package"
		}
	case "apt", "apt-get", "dnf", "yum", "zypper", "apk", "brew", "choco", "winget", "scoop":
		if packageManagerMutation(args) {
			return "Modify host packages"
		}
	case "curl":
		if externalHTTPMutation(args) {
			return "Modify remote HTTP resource"
		}
	}
	if name == "" {
		return "Run shell command"
	}
	return "Run " + filepath.Base(name) + " command"
}

func (m *Manager) validateProtectedShellAccess(cwd, command string, depth int) error {
	if m.protectedRoot == "" {
		return nil
	}
	if depth >= maxNestedShellDepth {
		return errors.New("control-plane state access denied: nested shell depth exceeded")
	}
	segments, err := splitShellSegments(command)
	if err != nil {
		return fmt.Errorf("control-plane state access denied: %w", err)
	}
	for _, segment := range segments {
		if m.referencesProtectedText(cwd, segment) {
			return errors.New("control-plane state access denied from MCP shell")
		}
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if inner, ok := nestedShellCommand(name, args); ok {
			if err := m.validateProtectedShellAccess(cwd, inner, depth+1); err != nil {
				return err
			}
		}
		for _, token := range tokens {
			if m.protectedShellToken(cwd, token) {
				return errors.New("control-plane state access denied from MCP shell")
			}
		}
	}
	return nil
}

func (m *Manager) referencesProtectedText(cwd, value string) bool {
	expanded := expandShellPathVariables(value)
	normalized, root := normalizeShellPathText(expanded), strings.TrimRight(normalizeShellPathText(m.protectedRoot), "/")
	if root != "" && strings.Contains(normalized, root+"/") {
		return true
	}
	for _, candidate := range absolutePathLiteral.FindAllString(expanded, -1) {
		if m.protectedShellToken(cwd, candidate) {
			return true
		}
	}
	for _, token := range strings.Fields(expanded) {
		if m.protectedShellToken(cwd, strings.Trim(token, `"'(),`)) {
			return true
		}
	}
	return false
}

func (m *Manager) protectedShellToken(cwd, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") {
		return false
	}
	if index := strings.IndexByte(value, '='); index > 0 && strings.HasPrefix(value, "-") {
		value = value[index+1:]
	}
	value = strings.Trim(value, `"'(),`)
	value = expandShellPathVariables(value)
	if strings.HasPrefix(value, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(value, "~"), `/\`))
		}
	}
	if index := strings.IndexAny(value, "*?["); index >= 0 {
		value = strings.TrimRight(value[:index], `/\`)
	}
	if value == "" || (!filepath.IsAbs(value) && !strings.ContainsAny(value, `/\`)) {
		return false
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}
	canonical, err := canonicalForContainment(value, false)
	return err == nil && m.protected(canonical)
}

func expandShellPathVariables(value string) string {
	value = os.ExpandEnv(value)
	if strings.Contains(value, "%") {
		value = windowsEnvReference.ReplaceAllStringFunc(value, func(match string) string {
			name := strings.Trim(match, "%")
			if env := os.Getenv(name); env != "" {
				return env
			}
			return match
		})
	}
	return value
}

func normalizeShellPathText(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, `\\`, "/"))
}

func (m *Manager) isMutationCommand(command string, depth int) bool {
	if isControlPlaneMutation(command, depth) {
		return true
	}
	if mutationWord.MatchString(command) {
		return true
	}
	targets, err := outputRedirectionTargets(command)
	if err != nil || len(targets) > 0 {
		return true
	}
	if depth >= maxNestedShellDepth {
		return false
	}
	segments, err := splitShellSegments(command)
	if err != nil {
		return true
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if name == "git" && isGitMutation(args) {
			return true
		}
		if _, ok := hostMutationReasonForInvocation(name, args); ok {
			return true
		}
		if _, ok := externalMutationReasonForInvocation(name, args); ok {
			return true
		}
		if mutationCommands[name] {
			return true
		}
		if pathMutationCommands[name] && isKnownPathMutation(name, args) {
			return true
		}
		if writeCommands[name] {
			return true
		}
		if inner, ok := nestedShellCommand(name, args); ok && m.isMutationCommand(inner, depth+1) {
			return true
		}
		if code, ok := inlineInterpreterCode(name, args); ok && inlineMutationAPI.MatchString(code) {
			return true
		}
	}
	return false
}

func isKnownPathMutation(name string, args []string) bool {
	switch name {
	case "chmod", "chown", "chgrp", "rsync":
		return true
	case "sed":
		return hasSedInPlace(args)
	case "perl":
		return hasPerlInPlace(args)
	case "dd":
		_, ok := assignmentValue(args, "of")
		return ok
	case "curl":
		return hasAnyOption(args, "-o", "--output", "--output-dir", "-O", "--remote-name")
	case "wget":
		return hasAnyOption(args, "-O", "--output-document", "-P", "--directory-prefix")
	default:
		return false
	}
}

func hasAnyOption(args []string, options ...string) bool {
	for _, arg := range args {
		for _, option := range options {
			if strings.EqualFold(arg, option) || strings.HasPrefix(strings.ToLower(arg), strings.ToLower(option)+"=") {
				return true
			}
		}
	}
	return false
}

func assignmentValue(args []string, key string) (string, bool) {
	prefix := strings.ToLower(key) + "="
	for _, arg := range args {
		if strings.HasPrefix(strings.ToLower(arg), prefix) {
			return arg[len(prefix):], true
		}
	}
	return "", false
}

func hasSedInPlace(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-i" || lower == "--in-place" || strings.HasPrefix(lower, "--in-place=") || (strings.HasPrefix(lower, "-i") && len(lower) > 2) {
			return true
		}
	}
	return false
}

func hasPerlInPlace(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Contains(strings.TrimPrefix(arg, "-"), "i") {
			return true
		}
	}
	return false
}

func isControlPlaneMutation(command string, depth int) bool {
	if depth >= maxNestedShellDepth {
		return false
	}
	segments, err := splitShellSegments(command)
	if err != nil {
		return false
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if isChatGPTMCPBinary(name) && !controlplane.IsReadOnlyArgs(args) {
			return true
		}
		if inner, ok := nestedShellCommand(name, args); ok && isControlPlaneMutation(inner, depth+1) {
			return true
		}
	}
	return false
}

func isChatGPTMCPBinary(name string) bool {
	switch strings.ToLower(strings.TrimSuffix(filepath.Base(name), ".exe")) {
	case "chatgpt-mcp", "cgm", "cmcp":
		return true
	default:
		return false
	}
}

func (m *Manager) validateWriteOperands(id, cwd, name string, args []string) error {
	if isPowerShellWriteCommand(name) {
		return m.validatePowerShellWriteOperands(id, cwd, name, args)
	}
	operands := make([]string, 0, len(args))
	optionsDone := false
	for _, arg := range args {
		if arg == "--" {
			optionsDone = true
			continue
		}
		if !optionsDone && strings.HasPrefix(arg, "-") {
			if index := strings.IndexByte(arg, '='); index > 0 {
				value := strings.TrimSpace(arg[index+1:])
				if value != "" && (looksLikePath(value) || unsafeShellPath(value)) {
					if err := m.validateLiteralPath(id, cwd, value, false); err != nil {
						return err
					}
				}
			}
			continue
		}
		operands = append(operands, arg)
	}
	minimum := 1
	if value, ok := writeMinimumOperands[name]; ok {
		minimum = value
	}
	if len(operands) < minimum {
		return fmt.Errorf("expected at least %d path operand(s)", minimum)
	}
	for _, operand := range operands {
		if isNullDevice(operand) {
			continue
		}
		if err := m.validateLiteralPath(id, cwd, operand, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) validatePowerShellWriteOperands(id, cwd, name string, args []string) error {
	required := 1
	if name == "copy-item" {
		required = 2
	}
	var paths []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		lower := strings.ToLower(arg)
		if index := strings.IndexByte(lower, '='); index > 0 && powerShellPathParameters[lower[:index]] {
			paths = append(paths, arg[index+1:])
			continue
		}
		if powerShellPathParameters[lower] {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", arg)
			}
			paths = append(paths, args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		positional = append(positional, arg)
	}
	for len(paths) < required && len(positional) > 0 {
		paths = append(paths, positional[0])
		positional = positional[1:]
	}
	if len(paths) < required {
		return fmt.Errorf("expected at least %d path operand(s)", required)
	}
	for _, value := range paths {
		if isNullDevice(value) {
			continue
		}
		if err := m.validateLiteralPath(id, cwd, value, false); err != nil {
			return err
		}
	}
	return nil
}

func isPowerShellWriteCommand(name string) bool {
	switch name {
	case "new-item", "set-content", "add-content", "out-file", "copy-item", "clear-content":
		return true
	default:
		return false
	}
}

func nestedShellCommand(name string, args []string) (string, bool) {
	switch strings.ToLower(name) {
	case "sh", "bash", "dash", "zsh", "fish":
		for i, arg := range args {
			lower := strings.ToLower(arg)
			if strings.HasPrefix(lower, "-") && strings.Contains(strings.TrimPrefix(lower, "-"), "c") && i+1 < len(args) {
				return args[i+1], true
			}
		}
	case "cmd", "cmd.exe":
		for i, arg := range args {
			if (strings.EqualFold(arg, "/c") || strings.EqualFold(arg, "/k")) && i+1 < len(args) {
				return args[i+1], true
			}
		}
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		for i, arg := range args {
			if (strings.EqualFold(arg, "-command") || strings.EqualFold(arg, "-c")) && i+1 < len(args) {
				return args[i+1], true
			}
		}
	}
	return "", false
}

func inlineInterpreterCode(name string, args []string) (string, bool) {
	switch strings.ToLower(name) {
	case "python", "python3", "py":
		return flagValue(args, "-c")
	case "node", "node.exe":
		if value, ok := flagValue(args, "-e"); ok {
			return value, true
		}
		return flagValue(args, "--eval")
	case "ruby", "perl":
		return flagValue(args, "-e")
	case "php":
		return flagValue(args, "-r")
	default:
		return "", false
	}
}

func flagValue(args []string, flag string) (string, bool) {
	for i, arg := range args {
		if strings.EqualFold(arg, flag) && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(strings.ToLower(arg), strings.ToLower(flag)+"=") {
			return arg[len(flag)+1:], true
		}
	}
	return "", false
}

func outputRedirectionTargets(command string) ([]string, error) {
	var targets []string
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if ch == '\\' && quote != '\'' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch != '>' {
			continue
		}
		j := i + 1
		if j < len(command) && command[j] == '>' {
			j++
		}
		if j < len(command) && command[j] == '|' {
			j++
		}
		for j < len(command) && (command[j] == ' ' || command[j] == '\t') {
			j++
		}
		if j < len(command) && command[j] == '(' {
			return nil, fmt.Errorf("dynamic output redirection cannot be proven workspace-safe")
		}
		if j < len(command) && command[j] == '&' {
			k := j + 1
			for k < len(command) && command[k] >= '0' && command[k] <= '9' {
				k++
			}
			if k > j+1 {
				i = k - 1
				continue
			}
		}
		if j >= len(command) {
			return nil, fmt.Errorf("output redirection is missing a target")
		}
		start := j
		if command[j] == '\'' || command[j] == '"' {
			targetQuote := command[j]
			j++
			start = j
			targetEscaped := false
			for j < len(command) {
				if targetEscaped {
					targetEscaped = false
					j++
					continue
				}
				if command[j] == '\\' && targetQuote != '\'' {
					targetEscaped = true
					j++
					continue
				}
				if command[j] == targetQuote {
					break
				}
				j++
			}
			if j >= len(command) {
				return nil, fmt.Errorf("unterminated quoted output redirection target")
			}
			targets = append(targets, command[start:j])
			i = j
			continue
		}
		for j < len(command) && !strings.ContainsRune(" \t\r\n;&|", rune(command[j])) {
			j++
		}
		if start == j {
			return nil, fmt.Errorf("output redirection is missing a target")
		}
		targets = append(targets, command[start:j])
		i = j - 1
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unbalanced shell quoting")
	}
	return targets, nil
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "~") || strings.HasPrefix(value, `\\`) {
		return true
	}
	slash := strings.ReplaceAll(value, `\`, "/")
	return slash == ".." || strings.HasPrefix(slash, "../") || strings.Contains(slash, "/../") || strings.HasSuffix(slash, "/..") || strings.HasPrefix(slash, "./") || strings.Contains(slash, "/")
}

func isNullDevice(value string) bool {
	clean := strings.ToLower(strings.TrimSpace(value))
	return clean == "/dev/null" || clean == "nul" || clean == `\\.\nul`
}
