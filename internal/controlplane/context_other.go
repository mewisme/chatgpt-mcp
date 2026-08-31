//go:build !linux

package controlplane

func ancestorToolContextActive() bool { return false }
