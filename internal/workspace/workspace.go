package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

type Workspace struct {
	ID   string
	Path string
}

func Resolve(path string) Workspace {
	clean, _ := filepath.Abs(path)
	hash := sha256.Sum256([]byte(strings.ToLower(clean)))
	return Workspace{ID: hex.EncodeToString(hash[:])[:16], Path: clean}
}
