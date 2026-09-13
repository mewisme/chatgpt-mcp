package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func New(prefix string, bytes int) (string, error) {
	if bytes <= 0 {
		return "", fmt.Errorf("id size must be positive")
	}
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	value := hex.EncodeToString(buffer)
	if prefix == "" {
		return value, nil
	}
	return prefix + "_" + value, nil
}

func Must(prefix string, bytes int) string {
	value, err := New(prefix, bytes)
	if err != nil {
		panic(err)
	}
	return value
}
