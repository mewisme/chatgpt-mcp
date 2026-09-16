package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func ReviewArgv(executable, requestID string) ([]string, error) {
	if err := validateRequestID(requestID); err != nil {
		return nil, err
	}
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return nil, errors.New("chatgpt-mcp executable is required")
	}
	return []string{executable, "tui", "requests", requestID}, nil
}

func WSLReviewArgv(terminal, distro, executable, requestID string) ([]string, error) {
	review, err := ReviewArgv(executable, requestID)
	if err != nil {
		return nil, err
	}
	terminal = strings.TrimSpace(terminal)
	if terminal == "" {
		return nil, errors.New("windows terminal is required")
	}
	if err := validateWSLDistro(distro); err != nil {
		return nil, err
	}
	return append([]string{terminal, "nt", "wsl.exe", "-d", distro, "--"}, review...), nil
}

func OpenApproval(ctx context.Context, requestID string) error {
	return openApproval(ctx, defaultHost(), os.Executable, requestID)
}

func openApproval(ctx context.Context, h host, executable func() (string, error), requestID string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	if abs, absErr := filepath.Abs(exe); absErr == nil {
		exe = abs
	}
	if h.runningOnWSL() {
		distro := strings.TrimSpace(h.env("WSL_DISTRO_NAME"))
		terminal, lookErr := h.lookup("wt.exe")
		if lookErr != nil {
			return lookErr
		}
		args, err := WSLReviewArgv(terminal, distro, exe, requestID)
		if err != nil {
			return err
		}
		return h.run(ctx, runSpec{Name: args[0], Args: args[1:]})
	}
	args, err := ReviewArgv(exe, requestID)
	if err != nil {
		return err
	}
	return h.run(ctx, runSpec{Name: args[0], Args: args[1:]})
}

func validateRequestID(id string) error {
	if strings.TrimSpace(id) != id || !strings.HasPrefix(id, "req_") || len(id) <= len("req_") {
		return errors.New("invalid approval request id")
	}
	for _, r := range id {
		if r > unicode.MaxASCII || !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return fmt.Errorf("invalid approval request id")
		}
	}
	return nil
}

func validateWSLDistro(name string) error {
	if strings.TrimSpace(name) != name || name == "" {
		return errors.New("invalid WSL distro name")
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !(r == '.' || r == '_' || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return errors.New("invalid WSL distro name")
		}
	}
	return nil
}
