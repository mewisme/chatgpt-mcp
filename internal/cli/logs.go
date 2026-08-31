package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type logsOptions struct {
	tail       int
	follow     bool
	all        bool
	showTime   bool
	noTime     bool
	since      string
	until      string
	session    string
	level      string
	components string
	workspace  string
	tool       string
	status     string
	source     string
	event      string
	grep       string
}

func logsCommand() *cobra.Command {
	options := &logsOptions{tail: 100, showTime: true}
	cmd := &cobra.Command{Use: "logs", Aliases: []string{"log"}, Short: "Read and follow structured runtime logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs(cmd, *options)
	}}
	addLogsFlags(cmd, options, true)
	addLogsCompletions(cmd)
	followOptions := &logsOptions{tail: 100, follow: true, showTime: true}
	follow := &cobra.Command{Use: "follow", Short: "Follow runtime logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs(cmd, *followOptions)
	}}
	addLogsFlags(follow, followOptions, false)
	addLogsCompletions(follow)
	pathCmd := &cobra.Command{Use: "path", Short: "Show the runtime journal path", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, args []string) {
		commandLogger(cmd).Notice("LOGS", "logs.path", runtimeevent.Path(config.RootPath()))
	}}
	var forceClear bool
	clear := &cobra.Command{Use: "clear", Short: "Clear runtime logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if !forceClear {
			return errors.New("refusing to clear runtime logs without --force")
		}
		if err := clearRuntimeLogs(cmd); err != nil {
			return err
		}
		commandLogger(cmd).Ready("LOGS", "logs.cleared", "Runtime logs cleared")
		return nil
	}}
	clear.Flags().BoolVar(&forceClear, "force", false, "clear current and rotated runtime logs")
	cmd.AddCommand(follow, pathCmd, clear)
	return cmd
}

func addLogsCompletions(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("session", completeSessionID)
	_ = cmd.RegisterFlagCompletionFunc("workspace", func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return workspaceCompletions(cmd, toComplete)
	})
	_ = cmd.RegisterFlagCompletionFunc("level", completeStatic("debug", "info", "warn", "error"))
	_ = cmd.RegisterFlagCompletionFunc("component", completeStatic("SERVER", "TUNNEL", "CONFIG", "MCP", "TOOL", "UPSTREAM", "SESSION", "SERVICE"))
}

func clearRuntimeLogs(cmd *cobra.Command) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
	_, running, err := managedRuntimeStatus(ctx)
	cancel()
	if err != nil {
		return err
	}
	if running {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		return requestRuntimeClearLogs(ctx)
	}
	journal, err := runtimeevent.NewJournal(config.RootPath(), runtimeevent.Options{})
	if err != nil {
		return err
	}
	return journal.Clear()
}

func addLogsFlags(cmd *cobra.Command, options *logsOptions, includeFollow bool) {
	cmd.Flags().IntVarP(&options.tail, "tail", "n", 100, "show the last N matching events; 0 shows all")
	if includeFollow {
		cmd.Flags().BoolVarP(&options.follow, "follow", "f", false, "follow new runtime events")
	}
	cmd.Flags().BoolVar(&options.noTime, "no-time", false, "hide timestamps from runtime event lines")
	cmd.Flags().StringVar(&options.since, "since", "", "show events since a duration such as 30m or an RFC3339 timestamp")
	cmd.Flags().StringVar(&options.until, "until", "", "show events through an RFC3339 timestamp")
	cmd.Flags().StringVar(&options.session, "session", "", "filter by session/run ID or displayed prefix")
	cmd.Flags().BoolVar(&options.all, "all", false, "show events from all sessions")
	cmd.MarkFlagsMutuallyExclusive("all", "session")
	cmd.Flags().StringVar(&options.level, "level", "", "minimum level: debug, info, warn, or error")
	cmd.Flags().StringVar(&options.components, "component", "", "comma-separated components such as SERVER,TUNNEL")
	cmd.Flags().StringVar(&options.workspace, "workspace", "", "workspace ID or registered workspace path")
	cmd.Flags().StringVar(&options.tool, "tool", "", "filter by tool name")
	cmd.Flags().StringVar(&options.status, "status", "", "filter by status")
	cmd.Flags().StringVar(&options.source, "source", "", "filter by source")
	cmd.Flags().StringVar(&options.event, "event", "", "filter by event-name glob")
	cmd.Flags().StringVar(&options.grep, "grep", "", "filter by case-insensitive text")
}

func runLogs(cmd *cobra.Command, options logsOptions) error {
	if options.noTime {
		options.showTime = false
	}
	query, err := buildLogsQuery(options)
	if err != nil {
		return err
	}
	visibility := logsVisibility(cmd)
	followCtx := cmd.Context()
	var interrupt *foregroundInterrupt
	var response *http.Response
	var followErr error
	if options.follow {
		interrupt = newForegroundInterrupt(cmd, true)
		defer interrupt.Close()
		followCtx = interrupt.Context
		response, _, followErr = openRuntimeEventStream(followCtx)
		if response != nil {
			defer response.Body.Close()
		}
	}
	allEvents, err := runtimeevent.Read(config.RootPath(), runtimeevent.Query{})
	if err != nil {
		return err
	}
	if !options.all && query.RunID == "" {
		query.RunID = latestRuntimeSession(allEvents)
	}
	events := matchingRuntimeEvents(allEvents, query)
	events = visibleRuntimeEvents(events, visibility)
	if options.tail > 0 && len(events) > options.tail {
		events = append([]runtimeevent.Event(nil), events[len(events)-options.tail:]...)
	}
	replay := newRuntimeReplay(cmd, options.showTime)
	lastByRun := map[string]uint64{}
	for _, event := range events {
		replay.Render(event)
		if event.RunID != "" && event.Sequence > lastByRun[event.RunID] {
			lastByRun[event.RunID] = event.Sequence
		}
	}
	if !options.follow {
		return nil
	}
	if followErr != nil {
		return fmt.Errorf("runtime is not running; cannot follow: %w", followErr)
	}
	return followRuntimeEventStream(followCtx, cmd, response, query, visibility, lastByRun, replay)
}

func latestRuntimeSession(events []runtimeevent.Event) string {
	for index := len(events) - 1; index >= 0; index-- {
		if runID := strings.TrimSpace(events[index].RunID); runID != "" {
			return runID
		}
	}
	return ""
}

func matchingRuntimeEvents(events []runtimeevent.Event, query runtimeevent.Query) []runtimeevent.Event {
	result := make([]runtimeevent.Event, 0, len(events))
	for _, event := range events {
		if query.Match(event) {
			result = append(result, event)
		}
	}
	return result
}

func buildLogsQuery(options logsOptions) (runtimeevent.Query, error) {
	if options.tail < 0 {
		return runtimeevent.Query{}, errors.New("tail must be zero or greater")
	}
	query := runtimeevent.Query{RunID: strings.TrimSpace(options.session), MinLevel: strings.ToLower(strings.TrimSpace(options.level)), Components: parseCSV(options.components), Tool: strings.TrimSpace(options.tool), Status: strings.TrimSpace(options.status), Source: strings.TrimSpace(options.source), EventGlob: strings.TrimSpace(options.event), Grep: strings.TrimSpace(options.grep)}
	if query.MinLevel != "" {
		switch query.MinLevel {
		case "debug", "info", "warn", "warning", "error":
		default:
			return runtimeevent.Query{}, errors.New("level must be debug, info, warn, or error")
		}
	}
	if query.EventGlob != "" {
		if _, err := path.Match(query.EventGlob, "test"); err != nil {
			return runtimeevent.Query{}, fmt.Errorf("invalid event glob: %w", err)
		}
	}
	if options.since != "" {
		value, err := parseLogsSince(options.since, time.Now())
		if err != nil {
			return runtimeevent.Query{}, err
		}
		query.Since = &value
	}
	if options.until != "" {
		value, err := parseLogTimestamp(options.until)
		if err != nil {
			return runtimeevent.Query{}, fmt.Errorf("invalid --until: %w", err)
		}
		query.Until = &value
	}
	if options.workspace != "" {
		workspaceID, err := resolveLogWorkspace(options.workspace)
		if err != nil {
			return runtimeevent.Query{}, err
		}
		query.WorkspaceID = workspaceID
	}
	return query, nil
}

func parseLogsSince(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if duration, err := time.ParseDuration(raw); err == nil {
		if duration < 0 {
			return time.Time{}, errors.New("--since duration must be positive")
		}
		return now.Add(-duration), nil
	}
	value, err := parseLogTimestamp(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since: use a duration such as 30m or RFC3339 timestamp: %w", err)
	}
	return value, nil
}

func parseLogTimestamp(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
		return value, nil
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(raw))
}

func resolveLogWorkspace(value string) (string, error) {
	value = strings.TrimSpace(value)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	if strings.HasPrefix(value, "ws_") {
		item, err := manager.Get(value)
		if err != nil {
			return "", err
		}
		return item.ID, nil
	}
	if strings.HasPrefix(value, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(value, "~"), `/\`))
		}
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = filepath.Clean(canonical)
	}
	items, err := manager.List()
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if filepath.Clean(item.Path) == absolute {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("workspace is not registered: %s", absolute)
}

func logsVisibility(cmd *cobra.Command) logger.Visibility {
	verbose, debug := commandLogMode(cmd)
	if debug {
		return logger.VisibilityDebug
	}
	if verbose {
		return logger.VisibilityVerbose
	}
	return logger.VisibilityDefault
}

func visibleRuntimeEvents(events []runtimeevent.Event, visibility logger.Visibility) []runtimeevent.Event {
	result := make([]runtimeevent.Event, 0, len(events))
	for _, event := range events {
		if event.Visibility <= visibility {
			result = append(result, event)
		}
	}
	return result
}

func logReplayLogger(cmd *cobra.Command, showTime bool) *logger.Logger {
	verbose, debug := commandLogMode(cmd)
	format, _ := commandLogFormat(cmd)
	timeMode := logger.TimeHide
	if showTime {
		timeMode = logger.TimeShow
	}
	return logger.NewWithOptions(logger.Options{Level: logger.Debug, Mode: logger.ModeFor(verbose, debug), Format: format, TimeMode: timeMode, Writer: commandLogWriter(cmd)})
}

func renderRuntimeEvent(log *logger.Logger, event runtimeevent.Event) {
	value := event.LoggerEvent()
	value.Time = event.Time.Local()
	log.Emit(value)
}

type runtimeReplay struct {
	log       *logger.Logger
	out       io.Writer
	format    logger.Format
	showTime  bool
	lastRunID string
}

func newRuntimeReplay(cmd *cobra.Command, showTime bool) *runtimeReplay {
	format, _ := commandLogFormat(cmd)
	return &runtimeReplay{log: logReplayLogger(cmd, showTime), out: commandLogWriter(cmd), format: format, showTime: showTime}
}

func (replay *runtimeReplay) Render(event runtimeevent.Event) {
	if replay == nil {
		return
	}
	if replay.format == logger.FormatText && event.RunID != "" && event.RunID != replay.lastRunID {
		replay.renderSessionHeader(event)
		replay.lastRunID = event.RunID
	}
	if replay.format == logger.FormatText && event.Name == "runtime.session.started" {
		return
	}
	renderRuntimeEvent(replay.log, event)
}

func (replay *runtimeReplay) renderSessionHeader(event runtimeevent.Event) {
	prefix := ""
	if replay.showTime && !event.Time.IsZero() {
		prefix = event.Time.Local().Format("2006-01-02 15:04:05") + " "
	}
	mode := "foreground"
	if event.Managed {
		mode = "managed"
		if event.ServiceScope != "" {
			mode += "/" + event.ServiceScope
		}
	}
	pid := ""
	if event.PID > 0 {
		pid = fmt.Sprintf(" · pid %d", event.PID)
	}
	line := fmt.Sprintf("%s── session %s%s · %s ──", prefix, shortSessionID(event.RunID), pid, mode)
	fmt.Fprintln(replay.out, cliDim(line))
}

func shortSessionID(value string) string {
	value = strings.TrimSpace(value)
	const max = 16
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func followRuntimeEventStream(ctx context.Context, cmd *cobra.Command, response *http.Response, query runtimeevent.Query, visibility logger.Visibility, lastByRun map[string]uint64, replay *runtimeReplay) error {
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	eventType := ""
	data := ""
	flush := func() error {
		defer func() { eventType, data = "", "" }()
		if eventType != "runtime" || strings.TrimSpace(data) == "" {
			return nil
		}
		var event runtimeevent.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("decode runtime event: %w", err)
		}
		if event.RunID != "" && event.Sequence <= lastByRun[event.RunID] {
			return nil
		}
		if !query.Match(event) || event.Visibility > visibility {
			return nil
		}
		replay.Render(event)
		if event.RunID != "" && event.Sequence > lastByRun[event.RunID] {
			lastByRun[event.RunID] = event.Sequence
		}
		return nil
	}
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if data != "" {
				data += "\n"
			}
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
