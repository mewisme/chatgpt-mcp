package cli

import (
	"io"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func addLoggingFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("verbose", false, "show additional runtime context")
	cmd.PersistentFlags().Bool("debug", false, "show full diagnostic logging")
	cmd.PersistentFlags().String("log-format", "text", "log output format: text or json")
}

func validateLoggingFlags(cmd *cobra.Command, _ []string) error {
	_, err := commandLogFormat(cmd)
	return err
}

func commandLogger(cmd *cobra.Command) *logger.Logger {
	verbose, debug := commandLogMode(cmd)
	format, _ := commandLogFormat(cmd)
	level := logger.Info
	if debug {
		level = logger.Debug
	}
	return logger.NewWithOptions(logger.Options{Level: level, Mode: logger.ModeFor(verbose, debug), Format: format, Writer: commandLogWriter(cmd)})
}

func commandLogMode(cmd *cobra.Command) (bool, bool) {
	flags := cmd.Root().PersistentFlags()
	verbose, _ := flags.GetBool("verbose")
	debug, _ := flags.GetBool("debug")
	return verbose, debug
}

func commandLogFormat(cmd *cobra.Command) (logger.Format, error) {
	value, _ := cmd.Root().PersistentFlags().GetString("log-format")
	return logger.ParseFormat(value)
}

func commandLogWriter(cmd *cobra.Command) io.Writer {
	if cmd == nil {
		return io.Discard
	}
	return cmd.OutOrStdout()
}
