//go:build linux

package secretstore

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

var errKernelKeyringUnavailable = errors.New("Linux kernel keyring unavailable")

type linuxBackend struct {
	durable Backend
	kernel  Backend
}

type kernelBackend struct{}

func newOSBackend() Backend { return linuxBackend{durable: keyringBackend{}, kernel: kernelBackend{}} }

func (b linuxBackend) Set(service, account, value string) error {
	durableErr := b.durable.Set(service, account, value)
	if durableErr == nil {
		return nil
	}
	if !secretServiceUnavailable(durableErr) {
		return durableErr
	}
	kernelErr := b.kernel.Set(service, account, value)
	if kernelErr == nil {
		return nil
	}
	return fmt.Errorf("Secret Service and Linux kernel keyring are unavailable: %w", errors.Join(durableErr, kernelErr))
}

func (b linuxBackend) Get(service, account string) (string, error) {
	value, durableErr := b.durable.Get(service, account)
	if durableErr == nil {
		return value, nil
	}
	durableMissing := errors.Is(durableErr, ErrNotFound)
	durableUnavailable := secretServiceUnavailable(durableErr)
	if !durableMissing && !durableUnavailable {
		return "", durableErr
	}
	value, kernelErr := b.kernel.Get(service, account)
	if kernelErr == nil {
		if durableMissing && b.durable.Set(service, account, value) == nil {
			_ = b.kernel.Delete(service, account)
		}
		return value, nil
	}
	if errors.Is(kernelErr, ErrNotFound) {
		return "", ErrNotFound
	}
	if durableMissing && errors.Is(kernelErr, errKernelKeyringUnavailable) {
		return "", ErrNotFound
	}
	if durableUnavailable && errors.Is(kernelErr, errKernelKeyringUnavailable) {
		return "", fmt.Errorf("Secret Service and Linux kernel keyring are unavailable: %w", errors.Join(durableErr, kernelErr))
	}
	return "", kernelErr
}

func (b linuxBackend) Delete(service, account string) error {
	durableErr := b.durable.Delete(service, account)
	if durableErr == nil {
		_ = b.kernel.Delete(service, account)
		return nil
	}
	durableMissing := errors.Is(durableErr, ErrNotFound)
	durableUnavailable := secretServiceUnavailable(durableErr)
	if !durableMissing && !durableUnavailable {
		return durableErr
	}
	kernelErr := b.kernel.Delete(service, account)
	if kernelErr == nil {
		return nil
	}
	if errors.Is(kernelErr, ErrNotFound) {
		return ErrNotFound
	}
	if durableMissing && errors.Is(kernelErr, errKernelKeyringUnavailable) {
		return ErrNotFound
	}
	if durableUnavailable && errors.Is(kernelErr, errKernelKeyringUnavailable) {
		return fmt.Errorf("Secret Service and Linux kernel keyring are unavailable: %w", errors.Join(durableErr, kernelErr))
	}
	return kernelErr
}

func (kernelBackend) Set(service, account, value string) error {
	description := kernelKeyDescription(service, account)
	id, err := kernelSearch(description)
	if errors.Is(err, ErrNotFound) {
		_, err = unix.AddKey("user", description, []byte(value), unix.KEY_SPEC_USER_KEYRING)
		return kernelError(err)
	}
	if err != nil {
		return err
	}
	_, err = unix.KeyctlBuffer(unix.KEYCTL_UPDATE, id, []byte(value), 0)
	return kernelError(err)
}

func (kernelBackend) Get(service, account string) (string, error) {
	id, err := kernelSearch(kernelKeyDescription(service, account))
	if err != nil {
		return "", err
	}
	size, err := unix.KeyctlBuffer(unix.KEYCTL_READ, id, nil, 0)
	if err != nil {
		return "", kernelError(err)
	}
	if size == 0 {
		return "", nil
	}
	buffer := make([]byte, size)
	length, err := unix.KeyctlBuffer(unix.KEYCTL_READ, id, buffer, 0)
	if err != nil {
		return "", kernelError(err)
	}
	if length > len(buffer) {
		return "", errors.New("Linux kernel keyring secret changed while reading")
	}
	return string(buffer[:length]), nil
}

func (kernelBackend) Delete(service, account string) error {
	id, err := kernelSearch(kernelKeyDescription(service, account))
	if err != nil {
		return err
	}
	_, err = unix.KeyctlInt(unix.KEYCTL_UNLINK, id, unix.KEY_SPEC_USER_KEYRING, 0, 0)
	return kernelError(err)
}

func kernelSearch(description string) (int, error) {
	id, err := unix.KeyctlSearch(unix.KEY_SPEC_USER_KEYRING, "user", description, 0)
	if err != nil {
		return 0, kernelError(err)
	}
	return id, nil
}

func kernelKeyDescription(service, account string) string { return service + "/" + account }

func kernelError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOKEY) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EKEYEXPIRED) || errors.Is(err, unix.EKEYREVOKED) {
		return ErrNotFound
	}
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
		return fmt.Errorf("%w: %v", errKernelKeyringUnavailable, err)
	}
	return err
}

func secretServiceUnavailable(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "org.freedesktop.secrets") && (strings.Contains(text, "not provided") || strings.Contains(text, "serviceunknown") || strings.Contains(text, "no owner")) ||
		strings.Contains(text, "dbus") && (strings.Contains(text, "not found") || strings.Contains(text, "not available") || strings.Contains(text, "no such file"))
}
