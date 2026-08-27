package tools

import (
	"encoding/base64"
	"os"
	"path/filepath"
)

type FileService struct{}

func resolve(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func (FileService) ReadText(workdir, path string) (string, error) {
	data, err := os.ReadFile(resolve(workdir, path))
	return string(data), err
}

func (FileService) ReadBase64(workdir, path string) (string, error) {
	data, err := os.ReadFile(resolve(workdir, path))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (FileService) WriteText(workdir, path, content string) error {
	file := resolve(workdir, path)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(content), 0644)
}

func (FileService) Delete(workdir, path string) error {
	return os.Remove(resolve(workdir, path))
}
