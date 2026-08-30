package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecRejectsOutputRedirectionOutsideWorkspace(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	_, err := manager.Exec(context.Background(), workspaceID, root, "echo escaped > "+outside)
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("error = %v, want workspace escape denial", err)
	}
	if _, statErr := os.Stat(outside); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside file was created or stat failed unexpectedly: %v", statErr)
	}
}

func TestBackgroundValidationRejectsWriteOutsideWorkspace(t *testing.T) {
	manager, workspaceID, root := newShellTestManager(t)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	_, err := manager.ValidateBackgroundCommand(workspaceID, root, "echo escaped > "+outside)
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("error = %v, want workspace escape denial", err)
	}
}

func TestWriteMutationRequiresWorkingDirectory(t *testing.T) {
	manager, workspaceID, _ := newShellTestManager(t)
	_, err := manager.Exec(context.Background(), workspaceID, "", "echo x > file.txt")
	if err == nil || !strings.Contains(err.Error(), "working_directory is required") {
		t.Fatalf("error = %v, want working_directory denial", err)
	}
}

func TestBackgroundWriteMutationRequiresWorkingDirectory(t *testing.T) {
	manager, workspaceID, _ := newShellTestManager(t)
	_, err := manager.ValidateBackgroundCommand(workspaceID, "", "touch file.txt")
	if err == nil || !strings.Contains(err.Error(), "working_directory is required") {
		t.Fatalf("error = %v, want working_directory denial", err)
	}
}
