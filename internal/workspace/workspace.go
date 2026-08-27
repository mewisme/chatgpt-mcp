package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

type Workspace struct { ID string; Path string }

func Resolve(path string) Workspace {
	clean, _ := filepath.Abs(path)
	sum := sha256.Sum256([]byte(clean))
	return Workspace{ID: hex.EncodeToString(sum[:])[:16], Path: clean}
}
