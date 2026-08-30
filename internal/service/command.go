package service

import (
	"fmt"
	"os/exec"
	"strings"
)

func runCommand(name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return text, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return text, nil
}

func commandSucceeded(name string, args ...string) (string, bool) {
	output, err := runCommand(name, args...)
	return output, err == nil
}
