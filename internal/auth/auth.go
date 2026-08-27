package auth

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/argon2"
)

func GenerateToken(prefix string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b)
}

func HashToken(token string) string {
	salt := []byte("chatgpt-mcp-auth")
	hash := argon2.IDKey([]byte(token), salt, 1, 64*1024, 4, 32)
	return base64.RawEncoding.EncodeToString(hash)
}

func VerifyToken(token, encoded string) bool {
	return strings.EqualFold(HashToken(token), encoded)
}
