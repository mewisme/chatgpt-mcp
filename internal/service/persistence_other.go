//go:build !linux

package service

func PersistenceWarning(Spec) string { return "" }
