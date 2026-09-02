//go:build !linux

package secretstore

func newOSBackend() Backend { return keyringBackend{} }
