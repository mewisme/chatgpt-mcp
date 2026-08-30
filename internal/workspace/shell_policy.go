package workspace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/controlplane"
)

const maxNestedShellDepth = 4

var (
	inlineMutationAPI = regexp.MustCompile(`(?i)(?:\bopen\s*\(|\b(?:write_text|write_bytes|writefile|writefilesync|appendfile|appendfilesync|createwritestream|unlink|unlinksync|rename|renamesync|copyfile|copyfilesync|mkdir|mkdirsync|rmdir|rmdirsync|truncate|remove|replace|rmtree|move)\s*\(|\bos\.system\s*\(|\bsubprocess\.|\bchild_process\b|\bexecsync\s*\(|\bspawnsync\s*\()`)
	writeCommands     = map[string]bool{
		"cp": true, "copy": true, "xcopy": true, "robocopy": true, "install": true, "touch": true, "mkdir": true, "md": true,
		"tee": true, "truncate": true, "ln": true, "link": true, "mkfifo": true,
		"new-item": true, "set-content": true, "add-content": true, "out-file": true, "copy-item": true,
	}
	writeMinimumOperands = map[string]int{
		"cp": 2, "copy": 2, "xcopy": 2, "robocopy": 2, "install": 2, "ln": 2, "link": 2, "copy-item": 2,
	}
	powerShellPathParameters = map[string]bool{
		"-path": true, "-literalpath": true, "-destination": true, "-filepath": true,
	}
)

func (m *Manager) ValidateShellCommand(id, workingDirectory, command string) error {
	if _, _, err := m.ResolveWorkingDirectory(id, workingDirectory); err != nil {
		return err
	}
	if isControlPlaneMutation(command, 0) {
		return fmt.Errorf("control-plane mutation denied from MCP shell: chatgpt-mcp configuration and permissions cannot be changed through shell tools")
	}
	if !m.IsMutationCommand(command) {
		return nil
	}
	return m.ValidateMutationCommand(id, workingDirectory, command)
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
	case "chatgpt-mcp", "cmcp":
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
	case "new-item", "set-content", "add-content", "out-file", "copy-item":
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
