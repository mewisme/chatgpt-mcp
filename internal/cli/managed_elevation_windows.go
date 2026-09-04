//go:build windows

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func elevateManagedCommand(*cobra.Command, string, string) error {
	return errors.New("system service scope is not supported on Windows; managed services use a per-user Scheduled Task")
}

func elevateManagedCommandWithBinary(*cobra.Command, string, string, string) error {
	return errors.New("system service scope is not supported on Windows; managed services use a per-user Scheduled Task")
}
