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
	"del": true, "erase": true, "remove-item": true, "move-item": true, "rename-item": true, "shred": true, "clear-content": true,
}

var destructiveMutationCommands = map[string]string{
	"rm": "filesystem deletion", "rmdir": "directory deletion", "unlink": "filesystem deletion", "del": "filesystem deletion", "erase": "filesystem deletion",
	"remove-item": "filesystem deletion", "shred": "irreversible file overwrite", "truncate": "file truncation", "clear-content": "file content deletion",
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

func destructiveMutationReason(command string) (string, bool) {
	segments, err := splitShellSegments(command)
	if err != nil {
		return "", false
	}
	for _, segment := range segments {
		tokens, err := shellWords(segment)
		if err != nil || len(tokens) == 0 {
			continue
		}
		name, args := commandName(tokens)
		if reason := destructiveMutationCommands[name]; reason != "" {
			return reason, true
		}
		if name == "find" && containsToken(args, "-delete") {
			return "recursive filesystem deletion", true
		}
		if name == "git" {
			if reason, ok := destructiveGitReason(args); ok {
				return reason, true
			}
		}
		switch name {
		case "sed":
			if hasSedInPlace(args) {
				return "in-place file overwrite", true
			}
		case "perl":
			if hasPerlInPlace(args) {
				return "in-place file overwrite", true
			}
		case "dd":
			if _, ok := assignmentValue(args, "of"); ok {
				return "raw file overwrite", true
			}
		case "rsync":
			if hasAnyOption(args, "--delete", "--delete-before", "--delete-during", "--delete-delay", "--delete-after", "--delete-excluded") {
				return "rsync destination deletion", true
			}
			if destination, ok := rsyncDestination(args); ok && looksRemotePath(destination) {
				return "remote rsync mutation", true
			}
		}
	}
	return "", false
}

func isGitMutation(args []string) bool {
	_, ok := destructiveGitReason(args)
	return ok
}

func destructiveGitReason(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	command := strings.ToLower(args[0])
	rest := args[1:]
	switch command {
	case "rm":
		return "Git tracked-file deletion", true
	case "clean":
		return "Git untracked-file deletion", true
	case "restore":
		return "Git working-tree overwrite", true
	case "reset":
		if containsAnyFold(rest, "--hard", "--merge", "--keep") {
			return "Git working-tree reset", true
		}
	case "checkout":
		if containsAnyFold(rest, "--", "-f", "--force") {
			return "Git working-tree overwrite", true
		}
	case "switch":
		if containsAnyFold(rest, "-f", "--force", "--discard-changes") {
			return "Git working-tree overwrite", true
		}
	case "stash":
		if len(rest) > 0 && (strings.EqualFold(rest[0], "drop") || strings.EqualFold(rest[0], "clear")) {
			return "Git stash deletion", true
		}
	case "branch":
		if containsAnyFold(rest, "-D", "--delete", "--force") {
			return "Git branch deletion", true
		}
	case "tag":
		if containsAnyFold(rest, "-d", "--delete") {
			return "Git tag deletion", true
		}
	case "push":
		return "remote Git mutation", true
	}
	return "", false
}

func containsAnyFold(values []string, targets ...string) bool {
	for _, value := range values {
		for _, target := range targets {
			if strings.EqualFold(value, target) {
				return true
			}
		}
	}
	return false
}

func (m *Manager) ValidateMutationCommand(id, baseDirectory, command string) error {
	_, cwd, err := m.ResolveDirectory(id, baseDirectory)
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
			if isGitMutation(args) {
				recognizedMutation = true
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
		if pathMutationCommands[name] && isKnownPathMutation(name, args) {
			recognizedMutation = true
			if err := m.validateKnownPathMutation(id, cwd, name, args); err != nil {
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
			if name == "clear-content" {
				if err := m.validatePowerShellWriteOperands(id, cwd, name, args); err != nil {
					return fmt.Errorf("mutation command denied: %s: %w", name, err)
				}
				continue
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

func (m *Manager) validateKnownPathMutation(id, cwd, name string, args []string) error {
	switch name {
	case "chmod", "chown", "chgrp":
		return m.validateMetadataMutation(id, cwd, args)
	case "sed":
		return m.validateSedInPlace(id, cwd, args)
	case "perl":
		return m.validatePerlInPlace(id, cwd, args)
	case "dd":
		value, ok := assignmentValue(args, "of")
		if !ok || strings.TrimSpace(value) == "" {
			return errors.New("dd output path is required")
		}
		return m.validateLiteralPath(id, cwd, value, false)
	case "rsync":
		return m.validateRsyncDestination(id, cwd, args)
	case "curl":
		return m.validateOptionPaths(id, cwd, args, map[string]bool{"-o": true, "--output": true, "--output-dir": true})
	case "wget":
		return m.validateOptionPaths(id, cwd, args, map[string]bool{"-o": true, "--output-document": true, "-p": true, "--directory-prefix": true})
	default:
		return nil
	}
}

func (m *Manager) validateMetadataMutation(id, cwd string, args []string) error {
	positionals := make([]string, 0, len(args))
	reference := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(strings.ToLower(arg), "--reference=") {
			if err := m.validateLiteralPath(id, cwd, arg[len("--reference="):], true); err != nil {
				return err
			}
			reference = true
			continue
		}
		if strings.EqualFold(arg, "--reference") {
			if i+1 >= len(args) {
				return errors.New("--reference requires a path")
			}
			if err := m.validateLiteralPath(id, cwd, args[i+1], true); err != nil {
				return err
			}
			reference = true
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		positionals = append(positionals, arg)
	}
	minimum := 2
	pathStart := 1
	if reference {
		minimum, pathStart = 1, 0
	}
	if len(positionals) < minimum {
		return errors.New("metadata mutation requires mode/owner and target path")
	}
	for _, path := range positionals[pathStart:] {
		if err := m.validateLiteralPath(id, cwd, path, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) validateSedInPlace(id, cwd string, args []string) error {
	files := scriptMutationFiles(args, true)
	if len(files) == 0 {
		return errors.New("sed in-place mutation requires a literal target path")
	}
	for _, path := range files {
		if err := m.validateLiteralPath(id, cwd, path, false); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) validatePerlInPlace(id, cwd string, args []string) error {
	files := scriptMutationFiles(args, false)
	if len(files) == 0 {
		return errors.New("perl in-place mutation requires a literal target path")
	}
	for _, path := range files {
		if err := m.validateLiteralPath(id, cwd, path, false); err != nil {
			return err
		}
	}
	return nil
}

func scriptMutationFiles(args []string, sed bool) []string {
	var files []string
	expressionProvided := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		lower := strings.ToLower(arg)
		if lower == "-e" || lower == "--expression" {
			expressionProvided = true
			i++
			continue
		}
		if strings.HasPrefix(lower, "--expression=") {
			expressionProvided = true
			continue
		}
		if lower == "-f" || lower == "--file" {
			expressionProvided = true
			i++
			continue
		}
		if strings.HasPrefix(lower, "--file=") || strings.HasPrefix(arg, "-") {
			continue
		}
		if !expressionProvided {
			expressionProvided = true
			continue
		}
		files = append(files, arg)
	}
	if !sed && len(files) == 0 && expressionProvided {
		return files
	}
	return files
}

func (m *Manager) validateRsyncDestination(id, cwd string, args []string) error {
	destination, ok := rsyncDestination(args)
	if !ok {
		return errors.New("rsync requires source and destination")
	}
	if looksRemotePath(destination) {
		return nil
	}
	return m.validateLiteralPath(id, cwd, destination, false)
}

func rsyncDestination(args []string) (string, bool) {
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if optionConsumesNext(strings.ToLower(arg), "--exclude-from", "--include-from", "--files-from", "--filter", "-e", "--rsh") {
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	if len(positionals) < 2 {
		return "", false
	}
	return positionals[len(positionals)-1], true
}

func (m *Manager) validateOptionPaths(id, cwd string, args []string, options map[string]bool) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		lower := strings.ToLower(arg)
		if options[lower] {
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", arg)
			}
			if err := m.validateLiteralPath(id, cwd, args[i+1], false); err != nil {
				return err
			}
			i++
			continue
		}
		for option := range options {
			if strings.HasPrefix(lower, option+"=") {
				if err := m.validateLiteralPath(id, cwd, arg[len(option)+1:], false); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func optionConsumesNext(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func looksRemotePath(value string) bool {
	if strings.Contains(value, "://") {
		return true
	}
	colon := strings.IndexByte(value, ':')
	return colon > 0 && !filepath.IsAbs(value)
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
		if token == "sudo" || token == "command" || token == "exec" || token == "nohup" {
			index++
			continue
		}
		if token == "env" {
			index++
			for index < len(tokens) {
				current := strings.ToLower(tokens[index])
				switch {
				case strings.Contains(tokens[index], "="):
					index++
				case current == "-i" || current == "--ignore-environment":
					index++
				case current == "-u" || current == "--unset":
					if index+1 >= len(tokens) {
						return "", nil
					}
					index += 2
				case strings.HasPrefix(current, "--unset="):
					index++
				default:
					goto envDone
				}
			}
		envDone:
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
