//go:build linux

package controlplane

import (
	"os"
	"strconv"
	"strings"
)

const maxToolContextAncestorDepth = 32

func ancestorToolContextActive() bool {
	pid := os.Getppid()
	for depth := 0; pid > 1 && depth < maxToolContextAncestorDepth; depth++ {
		if processEnvironmentHasToolContext(pid) {
			return true
		}
		next, ok := processParentPID(pid)
		if !ok || next <= 0 || next == pid {
			break
		}
		pid = next
	}
	return false
}

func processEnvironmentHasToolContext(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return false
	}
	prefix := ToolContextEnv + "="
	for _, entry := range strings.Split(string(data), "\x00") {
		if !strings.HasPrefix(entry, prefix) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(entry, prefix))) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func processParentPID(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
		return value, err == nil
	}
	return 0, false
}
