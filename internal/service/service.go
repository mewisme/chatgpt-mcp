package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
)

type Account struct {
	Username string
	UID      string
	GID      string
	HomeDir  string
}

type Spec struct {
	ID         string
	Scope      Scope
	ConfigRoot string
	Binary     string
	Account    Account
}

type Status struct {
	Installed bool
	Running   bool
	PID       int
	Backend   string
}

type Manager interface {
	Backend() string
	Install(Spec) error
	Start(Spec) error
	Stop(Spec) error
	Uninstall(Spec) error
	Status(Spec) (Status, error)
}

func NewSpec(configRoot, binary string, scope Scope, account Account) (Spec, error) {
	if strings.TrimSpace(configRoot) == "" {
		return Spec{}, errors.New("service config root is required")
	}
	absoluteRoot, err := filepath.Abs(configRoot)
	if err != nil {
		return Spec{}, err
	}
	binary, err = StableBinaryPath(binary)
	if err != nil {
		return Spec{}, err
	}
	return Spec{ID: ID(filepath.Clean(absoluteRoot), scope), Scope: scope, ConfigRoot: filepath.Clean(absoluteRoot), Binary: binary, Account: account}, nil
}

func ID(configRoot string, scope Scope) string {
	sum := sha256.Sum256([]byte(filepath.Clean(configRoot)))
	return "chatgpt-mcp-" + string(scope) + "-" + hex.EncodeToString(sum[:6])
}

func StableBinaryPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = os.Args[0]
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	resolved, err := exec.LookPath(value)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func Args(spec Spec) []string {
	return []string{"--config-dir", spec.ConfigRoot, "_service", "run", "--service-id", spec.ID, "--service-scope", string(spec.Scope)}
}
