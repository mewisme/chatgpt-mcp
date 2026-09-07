package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	ID              string
	Scope           Scope
	ConfigRoot      string
	Binary          string
	EnvironmentHash string
	Account         Account
}

type Status struct {
	Installed bool
	Running   bool
	PID       int
	Backend   string
}

type Manager interface {
	Backend() string
	DefinitionMatches(Spec) (bool, error)
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

func PrepareManagedBinary(configRoot, value string) (string, error) {
	binary, err := StableBinaryPath(value)
	if err != nil {
		return "", err
	}
	if !transientGoBuildBinary(binary) {
		return binary, nil
	}
	if strings.TrimSpace(configRoot) == "" {
		return "", errors.New("service config root is required")
	}
	hash, err := fileSHA256(binary)
	if err != nil {
		return "", err
	}
	name := filepath.Base(binary)
	destination := filepath.Join(configRoot, "runtime", "bin", "go-run", hash[:16], name)
	if info, statErr := os.Stat(destination); statErr == nil && !info.IsDir() {
		return filepath.Clean(destination), nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	if err := copyExecutableAtomic(binary, destination); err != nil {
		return "", err
	}
	return filepath.Clean(destination), nil
}

func transientGoBuildBinary(path string) bool {
	for _, part := range strings.FieldsFunc(filepath.Clean(path), func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.HasPrefix(strings.ToLower(part), "go-build") {
			return true
		}
	}
	return false
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyExecutableAtomic(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("managed service binary is a directory: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".chatgpt-mcp-bin-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	sourceFile, err := os.Open(source)
	if err != nil {
		_ = temp.Close()
		return err
	}
	_, copyErr := io.Copy(temp, sourceFile)
	closeSourceErr := sourceFile.Close()
	if copyErr != nil {
		_ = temp.Close()
		return copyErr
	}
	if closeSourceErr != nil {
		_ = temp.Close()
		return closeSourceErr
	}
	mode := info.Mode().Perm()
	if mode&0111 == 0 {
		mode |= 0700
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		if _, statErr := os.Stat(destination); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func Args(spec Spec) []string {
	return []string{"--config-dir", spec.ConfigRoot, "_service", "run", "--service-id", spec.ID, "--service-scope", string(spec.Scope), "--service-environment-hash", spec.EnvironmentHash}
}
