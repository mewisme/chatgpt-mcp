//go:build linux

package secretstore

import (
	"errors"
	"testing"
)

func TestLinuxBackendUsesDurableKeyringFirst(t *testing.T) {
	durable := newMemoryBackend()
	kernel := newMemoryBackend()
	if err := durable.Set("service", "account", "durable"); err != nil {
		t.Fatal(err)
	}
	if err := kernel.Set("service", "account", "kernel"); err != nil {
		t.Fatal(err)
	}
	value, err := (linuxBackend{durable: durable, kernel: kernel}).Get("service", "account")
	if err != nil || value != "durable" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestLinuxBackendPromotesKernelValueToDurableKeyring(t *testing.T) {
	durable := newMemoryBackend()
	kernel := newMemoryBackend()
	if err := kernel.Set("service", "account", "secret"); err != nil {
		t.Fatal(err)
	}
	value, err := (linuxBackend{durable: durable, kernel: kernel}).Get("service", "account")
	if err != nil || value != "secret" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if value, err := durable.Get("service", "account"); err != nil || value != "secret" {
		t.Fatalf("durable value=%q err=%v", value, err)
	}
	if _, err := kernel.Get("service", "account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kernel secret was not removed: %v", err)
	}
}

func TestLinuxBackendFallsBackToKernelWhenSecretServiceUnavailable(t *testing.T) {
	kernel := newMemoryBackend()
	backend := linuxBackend{durable: missingSecretServiceBackend{}, kernel: kernel}
	if err := backend.Set("service", "account", "secret"); err != nil {
		t.Fatal(err)
	}
	value, err := backend.Get("service", "account")
	if err != nil || value != "secret" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestLinuxBackendReportsNotFoundForLegacyMigrationOnHeadlessLinux(t *testing.T) {
	backend := linuxBackend{durable: missingSecretServiceBackend{}, kernel: newMemoryBackend()}
	if _, err := backend.Get("service", "account"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

type missingSecretServiceBackend struct{}

func (missingSecretServiceBackend) Set(string, string, string) error {
	return errors.New("The name org.freedesktop.secrets was not provided by any .service files")
}
func (missingSecretServiceBackend) Get(string, string) (string, error) {
	return "", errors.New("The name org.freedesktop.secrets was not provided by any .service files")
}
func (missingSecretServiceBackend) Delete(string, string) error {
	return errors.New("The name org.freedesktop.secrets was not provided by any .service files")
}
