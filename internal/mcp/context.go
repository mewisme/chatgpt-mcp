package mcp

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
)

type contextKey string

const workingDirectoryKey contextKey = "working_directory"

func WithWorkingDirectory(r *http.Request) (*http.Request, error) {
	value := r.Header.Get("X-Working-Directory")
	if value == "" {
		return nil, errors.New("working_directory is required")
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return nil, err
	}
	return r.WithContext(context.WithValue(r.Context(), workingDirectoryKey, path)), nil
}

func WorkingDirectory(ctx context.Context) string {
	value, _ := ctx.Value(workingDirectoryKey).(string)
	return value
}

func RequireWorkingDirectory(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := WithWorkingDirectory(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, req)
	})
}
