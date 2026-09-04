//go:build windows

package install

import (
	"bytes"
	"os"
	"strings"
)

const windowsAliasContent = "@echo off\r\nset \"CHATGPT_MCP_CLI_NAME=cgm\"\r\n\"%~dp0chatgpt-mcp.exe\" %*\r\n"

func statusAliasPlatform(layout Layout) (AliasStatus, error) {
	status := AliasStatus{State: AliasMissing, Path: layout.AliasPath, Target: layout.CurrentBinary}
	data, err := os.ReadFile(layout.AliasPath)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return AliasStatus{}, err
	}
	actual := strings.ReplaceAll(string(data), "\r\n", "\n")
	expected := strings.ReplaceAll(windowsAliasContent, "\r\n", "\n")
	if bytes.Equal([]byte(actual), []byte(expected)) {
		status.State = AliasInstalled
		return status, nil
	}
	status.State = AliasConflict
	return status, nil
}

func installAliasPlatform(layout Layout) error {
	file, err := os.OpenFile(layout.AliasPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(layout.AliasPath)
		}
	}()
	if _, err := file.WriteString(windowsAliasContent); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func removeAliasPlatform(layout Layout) error {
	return os.Remove(layout.AliasPath)
}
