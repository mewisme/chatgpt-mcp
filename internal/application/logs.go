package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type LogsQueryOptions struct {
	Tail       int
	All        bool
	Since      string
	Until      string
	Session    string
	Level      string
	Components string
	Workspace  string
	Tool       string
	Status     string
	Source     string
	Event      string
	Grep       string
}

type LogsSnapshot struct {
	Events         []runtimeevent.Event
	Query          runtimeevent.Query
	Session        string
	Total          int
	Truncated      bool
	LatestSequence map[string]uint64
}

type LogsInfo struct {
	Path  string
	Files int
	Bytes int64
}

func BuildLogsQuery(options LogsQueryOptions, now time.Time) (runtimeevent.Query, error) {
	if options.Tail < 0 {
		return runtimeevent.Query{}, errors.New("tail must be zero or greater")
	}
	query := runtimeevent.Query{RunID: strings.TrimSpace(options.Session), MinLevel: strings.ToLower(strings.TrimSpace(options.Level)), Components: splitLogCSV(options.Components), Tool: strings.TrimSpace(options.Tool), Status: strings.TrimSpace(options.Status), Source: strings.TrimSpace(options.Source), EventGlob: strings.TrimSpace(options.Event), Grep: strings.TrimSpace(options.Grep)}
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
	if strings.TrimSpace(options.Since) != "" {
		value, err := ParseLogsSince(options.Since, now)
		if err != nil {
			return runtimeevent.Query{}, err
		}
		query.Since = &value
	}
	if strings.TrimSpace(options.Until) != "" {
		value, err := ParseLogTimestamp(options.Until)
		if err != nil {
			return runtimeevent.Query{}, fmt.Errorf("invalid --until: %w", err)
		}
		query.Until = &value
	}
	if strings.TrimSpace(options.Workspace) != "" {
		workspaceID, err := ResolveLogWorkspace(options.Workspace)
		if err != nil {
			return runtimeevent.Query{}, err
		}
		query.WorkspaceID = workspaceID
	}
	return query, nil
}

func LoadLogs(options LogsQueryOptions, visibility logger.Visibility, bufferCap int, now time.Time) (LogsSnapshot, error) {
	query, err := BuildLogsQuery(options, now)
	if err != nil {
		return LogsSnapshot{}, err
	}
	allEvents, err := runtimeevent.Read(config.RootPath(), runtimeevent.Query{})
	if err != nil {
		return LogsSnapshot{}, err
	}
	if !options.All && query.RunID == "" {
		query.RunID = LatestRuntimeSession(allEvents)
	}
	latestSequence := map[string]uint64{}
	for _, event := range allEvents {
		if event.RunID != "" && event.Sequence > latestSequence[event.RunID] {
			latestSequence[event.RunID] = event.Sequence
		}
	}
	events := MatchLogs(allEvents, query, visibility)
	total := len(events)
	if options.Tail > 0 && len(events) > options.Tail {
		events = append([]runtimeevent.Event(nil), events[len(events)-options.Tail:]...)
	}
	truncated := false
	if bufferCap > 0 && len(events) > bufferCap {
		events = append([]runtimeevent.Event(nil), events[len(events)-bufferCap:]...)
		truncated = true
	}
	return LogsSnapshot{Events: events, Query: query, Session: query.RunID, Total: total, Truncated: truncated, LatestSequence: latestSequence}, nil
}

func MatchLogs(events []runtimeevent.Event, query runtimeevent.Query, visibility logger.Visibility) []runtimeevent.Event {
	result := make([]runtimeevent.Event, 0, len(events))
	for _, event := range events {
		if query.Match(event) && event.Visibility <= visibility {
			result = append(result, event)
		}
	}
	return result
}

func LatestRuntimeSession(events []runtimeevent.Event) string {
	for index := len(events) - 1; index >= 0; index-- {
		if runID := strings.TrimSpace(events[index].RunID); runID != "" {
			return runID
		}
	}
	return ""
}

func ParseLogsSince(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if duration, err := time.ParseDuration(raw); err == nil {
		if duration < 0 {
			return time.Time{}, errors.New("--since duration must be positive")
		}
		return now.Add(-duration), nil
	}
	value, err := ParseLogTimestamp(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since: use a duration such as 30m or RFC3339 timestamp: %w", err)
	}
	return value, nil
}

func ParseLogTimestamp(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err == nil {
		return value, nil
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(raw))
}

func ResolveLogWorkspace(value string) (string, error) {
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

func LogFields(event runtimeevent.Event, visibility logger.Visibility) []runtimeevent.Field {
	fields := make([]runtimeevent.Field, 0, len(event.Fields))
	for _, field := range event.Fields {
		if field.Visibility <= visibility {
			fields = append(fields, field)
		}
	}
	return fields
}

func LoadLogsInfo() (LogsInfo, error) {
	files, err := runtimeevent.FilesOldestFirst(config.RootPath())
	if err != nil {
		return LogsInfo{}, err
	}
	var bytes int64
	for _, file := range files {
		if info, statErr := os.Stat(file); statErr == nil {
			bytes += info.Size()
		} else if !os.IsNotExist(statErr) {
			return LogsInfo{}, statErr
		}
	}
	return LogsInfo{Path: runtimeevent.Path(config.RootPath()), Files: len(files), Bytes: bytes}, nil
}

func ClearLogs(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := runtimecontrol.Request(requestCtx, http.MethodPost, "/logs/clear", nil, &map[string]bool{})
	if err == nil {
		return nil
	}
	if !runtimecontrol.IsUnavailable(err) {
		return err
	}
	journal, journalErr := runtimeevent.NewJournal(config.RootPath(), runtimeevent.Options{})
	if journalErr != nil {
		return journalErr
	}
	return journal.Clear()
}

func splitLogCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}
