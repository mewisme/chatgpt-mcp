package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
)

var mutationWord = regexp.MustCompile(`(?i)(^|[^a-z0-9_.-])(rm|rmdir|unlink|mv|rename|del|erase|move|ren|remove-item|move-item|rename-item)([^a-z0-9_.-]|$)|\bgit\s+(?:mv|rm|clean)\b|\bfind\b[\s\S]*\s-delete\b|\b(?:os\.(?:remove|unlink|rename|replace)|shutil\.(?:move|rmtree)|fs\.(?:unlink|rm|rename))\b`)

var cwdCommands = map[string]bool{
	"cd": true, "pushd": true, "popd": true, "chdir": true, "set-location": true, "sl": true,
}

var mutationCommands = map[string]bool{
	"rm": true, "rmdir": true, "unlink": true, "mv": true, "move": true, "ren": true, "rename": true,
	"del": true, "erase": true, "remove-item": true, "move-item": true, "rename-item": true,
}

var longMutationOptions = map[string]bool{
	"--force": true, "--recursive": true, "--verbose": true, "--interactive": true, "--no-clobber": true,
	"--dir": true, "--quiet": true, "--cached": true, "--ignore-unmatch": true,
	"-force": true, "-recurse": true, "-verbose": true, "-confirm:$false": true, "-whatif:$false": true,
	"-path": true, "-literalpath": true, "-destination": true,
}

func (m *Manager) IsMutationCommand(command string) bool {
	return m.isMutationCommand(command, 0)
}

func (m *Manager) ValidateMutationCommand(id, workingDirectory, command string) error {
	_, cwd, err := m.ResolveWorkingDirectory(id, workingDirectory)
	if err != nil {
		return err
	}
	if !m.IsMutationCommand(command) {
		return nil
	}

	segments, err := splitShellSegments(command)
	if err != nil {
		return fmt.Errorf("mutation command denied: %w", err)
	}

	redirections, err := outputRedirectionTargets(command)
	if err != nil {
		return fmt.Errorf("mutation command denied: %w", err)
	}
	recognizedMutation := false
	for _, target := range redirections {
		recognizedMutation = true
		if isNullDevice(target) {
			continue
		}
		if err := m.validateLiteralPath(id, cwd, target, false); err != nil {
			return fmt.Errorf("mutation command denied: output redirection: %w", err)
		}
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil {
			return fmt.Errorf("mutation command denied: %w", err)
		}
		if len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if inner, ok := nestedShellCommand(name, args); ok && m.isMutationCommand(inner, 1) {
			return fmt.Errorf("mutation command denied: nested %s mutation cannot be proven workspace-safe", name)
		}
		if code, ok := inlineInterpreterCode(name, args); ok && inlineMutationAPI.MatchString(code) {
			return fmt.Errorf("mutation command denied: inline %s mutation cannot be proven workspace-safe", name)
		}
		if cwdCommands[name] {
			if name == "popd" {
				return errors.New("mutation command denied: popd cannot be proven workspace-safe")
			}
			if name == "pushd" && len(args) == 0 {
				return errors.New("mutation command denied: pushd requires an explicit target")
			}
			target := "."
			if len(args) > 0 {
				target = args[0]
			}
			resolved, err := m.ResolvePath(id, cwd, target, true)
			if err != nil {
				return fmt.Errorf("mutation command denied: cwd change target is invalid: %w", err)
			}
			if resolved != cwd {
				return fmt.Errorf("mutation command denied: cwd change from %s to %s", cwd, resolved)
			}
			continue
		}

		if name == "git" && len(args) > 0 {
			switch strings.ToLower(args[0]) {
			case "mv":
				recognizedMutation = true
				if err := m.validateLiteralOperands(id, cwd, args[1:], 2); err != nil {
					return fmt.Errorf("mutation command denied: git mv: %w", err)
				}
				continue
			case "rm":
				recognizedMutation = true
				if err := m.validateLiteralOperands(id, cwd, args[1:], 1); err != nil {
					return fmt.Errorf("mutation command denied: git rm: %w", err)
				}
				continue
			case "clean":
				recognizedMutation = true
				for _, arg := range args[1:] {
					if !strings.HasPrefix(arg, "-") {
						if err := m.validateLiteralPath(id, cwd, arg, false); err != nil {
							return fmt.Errorf("mutation command denied: git clean: %w", err)
						}
					}
				}
				continue
			}
		}
		if name == "find" && containsToken(args, "-delete") {
			recognizedMutation = true
			roots := findRoots(args)
			if len(roots) == 0 {
				roots = []string{"."}
			}
			for _, root := range roots {
				if err := m.validateLiteralPath(id, cwd, root, true); err != nil {
					return fmt.Errorf("mutation command denied: find -delete: %w", err)
				}
			}
			continue
		}
		if writeCommands[name] {
			recognizedMutation = true
			if err := m.validateWriteOperands(id, cwd, name, args); err != nil {
				return fmt.Errorf("mutation command denied: %s: %w", name, err)
			}
			continue
		}
		if mutationCommands[name] {
			recognizedMutation = true
			minimum := 1
			if name == "mv" || name == "move" || name == "ren" || name == "rename" || name == "move-item" || name == "rename-item" {
				minimum = 2
			}
			if err := m.validateLiteralOperands(id, cwd, args, minimum); err != nil {
				return fmt.Errorf("mutation command denied: %s: %w", name, err)
			}
		}
	}

	if !recognizedMutation {
		return errors.New("mutation command denied: destructive/rename operation cannot be proven workspace-safe")
	}
	return nil
}

func (m *Manager) validateLiteralOperands(id, cwd string, args []string, minimum int) error {
	operands := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if !allowedMutationOption(arg) {
				return fmt.Errorf("unsupported option %q", arg)
			}
			continue
		}
		operands = append(operands, arg)
	}
	if len(operands) < minimum {
		return fmt.Errorf("expected at least %d literal path operand(s)", minimum)
	}
	for _, operand := range operands {
		if err := m.validateLiteralPath(id, cwd, operand, false); err != nil {
			return err
		}
	}
	return nil
}

func allowedMutationOption(value string) bool {
	lower := strings.ToLower(value)
	if longMutationOptions[lower] {
		return true
	}
	if !strings.HasPrefix(value, "-") || strings.HasPrefix(value, "--") || len(value) < 2 {
		return false
	}
	for _, flag := range value[1:] {
		if !strings.ContainsRune("frRvinTdq", flag) {
			return false
		}
	}
	return true
}

func (m *Manager) validateLiteralPath(id, cwd, value string, mustExist bool) error {
	if unsafeShellPath(value) {
		return fmt.Errorf("dynamic path %q is not allowed", value)
	}
	path := value
	if index := strings.IndexAny(path, "*?["); index >= 0 {
		path = path[:index]
		path = strings.TrimRight(path, `/\`)
		if path == "" {
			path = "."
		} else if filepath.Base(path) != "." {
			path = filepath.Dir(path)
		}
		mustExist = true
	}
	if _, err := m.ResolvePath(id, cwd, path, mustExist); err != nil {
		return err
	}
	return nil
}

func unsafeShellPath(value string) bool {
	return strings.ContainsAny(value, "$`{}<>|;&\n\r") || strings.HasPrefix(value, "~") || strings.Contains(value, "$(")
}

func commandName(tokens []string) (string, []string) {
	index := 0
	for index < len(tokens) {
		token := strings.ToLower(filepath.Base(tokens[index]))
		if token == "sudo" || token == "command" {
			index++
			continue
		}
		if token == "env" {
			index++
			for index < len(tokens) && strings.Contains(tokens[index], "=") {
				index++
			}
			continue
		}
		if strings.Contains(tokens[index], "=") && !strings.ContainsAny(tokens[index], `/\`) {
			index++
			continue
		}
		if index >= len(tokens) {
			return "", nil
		}
		return strings.ToLower(filepath.Base(tokens[index])), tokens[index+1:]
	}
	return "", nil
}

func splitShellSegments(command string) ([]string, error) {
	var segments []string
	var current strings.Builder
	var quote rune
	escaped := false
	for i, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if r == ';' || r == '\n' || r == '\r' || r == '&' || r == '|' {
			if text := strings.TrimSpace(current.String()); text != "" {
				segments = append(segments, text)
			}
			current.Reset()
			if (r == '&' || r == '|') && i+1 < len(command) {
				continue
			}
			continue
		}
		current.WriteRune(r)
	}
	if quote != 0 || escaped {
		return nil, errors.New("unbalanced shell quoting")
	}
	if text := strings.TrimSpace(current.String()); text != "" {
		segments = append(segments, text)
	}
	return segments, nil
}

func shellWords(segment string) ([]string, error) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, r := range segment {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' && runtime.GOOS != "windows" {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if quote != 0 || escaped {
		return nil, errors.New("unbalanced shell quoting")
	}
	flush()
	return words, nil
}

func containsToken(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func findRoots(args []string) []string {
	var roots []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || arg == "!" || arg == "(" {
			break
		}
		roots = append(roots, arg)
	}
	return roots
}
