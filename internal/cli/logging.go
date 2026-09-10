package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
)

const machineOutputAnnotation = "chatgpt-mcp.machine-output"

var commandLoggers sync.Map

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
	if cmd != nil {
		if value, ok := commandLoggers.Load(cmd); ok {
			return value.(*logger.Logger)
		}
	}
	verbose, debug := commandLogMode(cmd)
	format, _ := commandLogFormat(cmd)
	level := logger.Info
	if debug {
		level = logger.Debug
	}
	created := logger.NewWithOptions(logger.Options{Level: level, Mode: logger.ModeFor(verbose, debug), Format: format, Writer: commandLogWriter(cmd)})
	if cmd == nil {
		return created
	}
	value, loaded := commandLoggers.LoadOrStore(cmd, created)
	if loaded {
		created.Close()
		return value.(*logger.Logger)
	}
	return created
}

func closeCommandLogger(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if value, ok := commandLoggers.LoadAndDelete(cmd); ok {
		value.(*logger.Logger).Close()
	}
}

func startCommandSpinner(cmd *cobra.Command, log *logger.Logger, component, name, message string) {
	format, err := commandLogFormat(cmd)
	verbose, debug := commandLogMode(cmd)
	if err == nil && format == logger.FormatText && !verbose && !debug && logger.CanAnimate(commandLogWriter(cmd)) {
		log.Action(component, name, message)
	}
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
	if commandMachineOutput(cmd) {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}

func addJSONOutputFlag(cmd *cobra.Command, target *bool) {
	cmd.Flags().BoolVar(target, "json", false, "print JSON")
	markMachineOutput(cmd, "json")
}

func markMachineOutput(cmd *cobra.Command, mode string) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[machineOutputAnnotation] = mode
}

func commandMachineOutput(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	switch cmd.Annotations[machineOutputAnnotation] {
	case "always":
		return true
	case "json":
		flag := cmd.Flags().Lookup("json")
		return flag != nil && flag.Value.String() == "true"
	default:
		return false
	}
}

func logCommandStart(cmd *cobra.Command, args []string) {
	log := commandLogger(cmd)
	log.Verbose("CLI", "cli.command.starting", "Executing command",
		logger.WithVerbose("command", cmd.CommandPath()),
		logger.WithVerbose("cwd", currentWorkingDirectory()),
		logger.WithVerbose("pid", os.Getpid()),
	)
	log.Diagnostic(logger.Info, "CLI", "cli.command.context", "Command context",
		logger.WithDebug("pid", os.Getpid()),
		logger.WithDebug("cwd", currentWorkingDirectory()),
		logger.WithDebug("config", config.RootPath()),
		logger.WithDebug("arg_count", len(args)),
		logger.WithDebug("changed_flags", commandChangedFlags(cmd)),
	)
}

func logCommandCompleted(cmd *cobra.Command, started time.Time) {
	commandLogger(cmd).Verbose("CLI", "cli.command.completed", "Command completed",
		logger.WithVerbose("command", cmd.CommandPath()),
		logger.WithVerbose("duration_ms", time.Since(started).Milliseconds()),
	)
}

func logCommandFailure(cmd *cobra.Command, err error, started time.Time) {
	if cmd == nil {
		return
	}
	commandLogger(cmd).Failure("CLI", "cli.command.failed", "Command failed", err,
		logger.WithVerbose("command", cmd.CommandPath()),
		logger.WithVerbose("duration_ms", time.Since(started).Milliseconds()),
		logger.WithDebug("pid", os.Getpid()),
		logger.WithDebug("cwd", currentWorkingDirectory()),
		logger.WithDebug("config", config.RootPath()),
		logger.WithDebug("changed_flags", commandChangedFlags(cmd)),
		logger.WithDebug("error_type", fmt.Sprintf("%T", err)),
		logger.WithDebug("error_chain", commandErrorChain(err)),
	)
}

func commandChangedFlags(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	seen := map[string]bool{}
	values := []string{}
	visit := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		values = append(values, "--"+name)
	}
	cmd.Flags().Visit(func(flag *pflag.Flag) { visit(flag.Name) })
	cmd.InheritedFlags().Visit(func(flag *pflag.Flag) { visit(flag.Name) })
	cmd.Root().PersistentFlags().Visit(func(flag *pflag.Flag) { visit(flag.Name) })
	sort.Strings(values)
	return values
}

func commandErrorChain(err error) []string {
	values := []string{}
	var walk func(error, int)
	walk = func(current error, depth int) {
		if current == nil || depth >= 32 {
			return
		}
		values = append(values, fmt.Sprintf("%T: %v", current, current))
		switch typed := current.(type) {
		case interface{ Unwrap() []error }:
			for _, nested := range typed.Unwrap() {
				walk(nested, depth+1)
			}
		case interface{ Unwrap() error }:
			walk(typed.Unwrap(), depth+1)
		}
	}
	walk(err, 0)
	return values
}

func currentWorkingDirectory() string {
	value, err := os.Getwd()
	if err != nil {
		return "<unavailable>"
	}
	return value
}

func logCommandStep(cmd *cobra.Command, component, name, message string, fields ...logger.Field) {
	commandLogger(cmd).Verbose(component, name, message, fields...)
}

func logCommandDebug(cmd *cobra.Command, component, name, message string, fields ...logger.Field) {
	commandLogger(cmd).Diagnostic(logger.Info, component, name, message, fields...)
}

func commandTraceObserver(cmd *cobra.Command) tracepkg.Observer {
	if cmd == nil {
		return nil
	}
	return func(event tracepkg.Event) {
		fields := make([]logger.Field, 0, len(event.Fields)+1)
		fields = append(fields, logger.WithDebug("trace_phase", event.Phase))
		for _, field := range event.Fields {
			fields = append(fields, logger.WithVerbose(field.Key, field.Value))
		}
		commandLogger(cmd).Verbose(event.Component, event.Name, event.Message, fields...)
	}
}
