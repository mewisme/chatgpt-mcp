package tools

import (
	"os"
	"path/filepath"
)

type ReadFilesService struct{}

type ReadFile struct { Path string `json:"path"`; Content string `json:"content"` }

func (ReadFilesService) Read(ctx Context, paths []string) ([]ReadFile, error) {
	result := make([]ReadFile, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) { path = filepath.Join(ctx.WorkingDirectory, path) }
		data, err := os.ReadFile(path)
		if err != nil { return nil, err }
		result = append(result, ReadFile{Path: path, Content: string(data)})
	}
	return result, nil
}
