package secretstore

import (
	"errors"

	keyring "github.com/zalando/go-keyring"
)

type keyringBackend struct{}

func (keyringBackend) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}
func (keyringBackend) Get(service, account string) (string, error) {
	value, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return value, err
}
func (keyringBackend) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
