package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
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
	since      string
	until      string
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
	options := &logsOptions{tail: 100}
	cmd := &cobra.Command{Use: "logs", Aliases: []string{"log"}, Short: "Read and follow structured runtime logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs(cmd, *options)
	}}
	addLogsFlags(cmd, options, true)
	followOptions := &logsOptions{tail: 100, follow: true}
	follow := &cobra.Command{Use: "follow", Short: "Follow runtime logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs(cmd, *followOptions)
	}}
	addLogsFlags(follow, followOptions, false)
	cmd.AddCommand(follow)
	return cmd
}

func addLogsFlags(cmd *cobra.Command, options *logsOptions, includeFollow bool) {
	cmd.Flags().IntVarP(&options.tail, "tail", "n", 100, "show the last N matching events; 0 shows all")
	if includeFollow {
		cmd.Flags().BoolVarP(&options.follow, "follow", "f", false, "follow new runtime events")
	}
	cmd.Flags().StringVar(&options.since, "since", "", "show events since a duration such as 30m or an RFC3339 timestamp")
	cmd.Flags().StringVar(&options.until, "until", "", "show events through an RFC3339 timestamp")
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
	query, err := buildLogsQuery(options)
	if err != nil {
		return err
	}
	visibility := logsVisibility(cmd)
	var response *http.Response
	var followErr error
	if options.follow {
		response, _, followErr = openRuntimeEventStream(cmd.Context())
		if response != nil {
			defer response.Body.Close()
		}
	}
	events, err := runtimeevent.Read(config.RootPath(), query)
	if err != nil {
		return err
	}
	events = visibleRuntimeEvents(events, visibility)
	if options.tail > 0 && len(events) > options.tail {
		events = append([]runtimeevent.Event(nil), events[len(events)-options.tail:]...)
	}
	replay := logReplayLogger(cmd)
	lastByRun := map[string]uint64{}
	for _, event := range events {
		renderRuntimeEvent(replay, event)
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
	return followRuntimeEventStream(cmd, response, query, visibility, lastByRun, replay)
}

func buildLogsQuery(options logsOptions) (runtimeevent.Query, error) {
	if options.tail < 0 {
		return runtimeevent.Query{}, errors.New("tail must be zero or greater")
	}
	query := runtimeevent.Query{MinLevel: strings.ToLower(strings.TrimSpace(options.level)), Components: parseCSV(options.components), Tool: strings.TrimSpace(options.tool), Status: strings.TrimSpace(options.status), Source: strings.TrimSpace(options.source), EventGlob: strings.TrimSpace(options.event), Grep: strings.TrimSpace(options.grep)}
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

func logReplayLogger(cmd *cobra.Command) *logger.Logger {
	verbose, debug := commandLogMode(cmd)
	format, _ := commandLogFormat(cmd)
	return logger.NewWithOptions(logger.Options{Level: logger.Debug, Mode: logger.ModeFor(verbose, debug), Format: format, Writer: commandLogWriter(cmd)})
}

func renderRuntimeEvent(log *logger.Logger, event runtimeevent.Event) {
	value := event.LoggerEvent()
	value.Time = event.Time.Local()
	log.Emit(value)
}

func followRuntimeEventStream(cmd *cobra.Command, response *http.Response, query runtimeevent.Query, visibility logger.Visibility, lastByRun map[string]uint64, replay *logger.Logger) error {
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
		renderRuntimeEvent(replay, event)
		if event.RunID != "" && event.Sequence > lastByRun[event.RunID] {
			lastByRun[event.RunID] = event.Sequence
		}
		return nil
	}
	for scanner.Scan() {
		select {
		case <-cmd.Context().Done():
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
	if err := scanner.Err(); err != nil && cmd.Context().Err() == nil {
		return err
	}
	return nil
}
