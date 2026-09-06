package page

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type logsFilterFormData struct {
	Tail       string
	All        bool
	Session    string
	Since      string
	Until      string
	Visibility string
	Level      string
	Components string
	Workspace  string
	Tool       string
	Status     string
	Source     string
	Event      string
	Grep       string
}

func newLogsFilterForm(options application.LogsQueryOptions, visibility logger.Visibility) (component.Form, *logsFilterFormData) {
	level := options.Level
	if strings.TrimSpace(level) == "" {
		level = "all"
	}
	data := &logsFilterFormData{
		Tail: strconv.Itoa(options.Tail), All: options.All, Session: options.Session, Since: options.Since, Until: options.Until, Visibility: logsVisibilityValue(visibility), Level: level,
		Components: options.Components, Workspace: options.Workspace, Tool: options.Tool, Status: options.Status, Source: options.Source, Event: options.Event, Grep: options.Grep,
	}
	form := component.NewForm(
		component.Group(
			component.Input("Tail", &data.Tail).Validate(func(value string) error {
				n, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil || n < 0 {
					return fmt.Errorf("tail must be zero or greater")
				}
				return nil
			}),
			component.Switch("All sessions", &data.All),
			component.Input("Session", &data.Session),
			component.Input("Since", &data.Since).Description("Duration such as 30m or RFC3339 timestamp"),
			component.Input("Until", &data.Until).Description("RFC3339 timestamp"),
		).Title("Range"),
		component.Group(
			component.Select("Visibility", &data.Visibility,
				huh.NewOption("Normal", "normal"), huh.NewOption("Verbose", "verbose"), huh.NewOption("Debug", "debug")),
			component.Select("Minimum level", &data.Level,
				huh.NewOption("All", "all"), huh.NewOption("Debug", "debug"), huh.NewOption("Info", "info"), huh.NewOption("Warn", "warn"), huh.NewOption("Error", "error")),
			component.Input("Components", &data.Components).Description("Comma-separated, e.g. SERVER,TOOL"),
			component.Input("Workspace", &data.Workspace).Description("Workspace ID or registered workspace path"),
			component.Input("Tool", &data.Tool),
			component.Input("Status", &data.Status),
			component.Input("Source", &data.Source),
			component.Input("Event glob", &data.Event),
			component.Input("Grep", &data.Grep),
		).Title("Filters"),
	)
	return form, data
}

func (data *logsFilterFormData) Options() (application.LogsQueryOptions, logger.Visibility, error) {
	if data == nil {
		return application.LogsQueryOptions{}, logger.VisibilityDefault, fmt.Errorf("log filters are unavailable")
	}
	tail, err := strconv.Atoi(strings.TrimSpace(data.Tail))
	if err != nil || tail < 0 {
		return application.LogsQueryOptions{}, logger.VisibilityDefault, fmt.Errorf("tail must be zero or greater")
	}
	if data.All && strings.TrimSpace(data.Session) != "" {
		return application.LogsQueryOptions{}, logger.VisibilityDefault, fmt.Errorf("all sessions and session filter cannot be used together")
	}
	visibility, err := parseLogsVisibility(data.Visibility)
	if err != nil {
		return application.LogsQueryOptions{}, logger.VisibilityDefault, err
	}
	level := strings.TrimSpace(data.Level)
	if level == "all" {
		level = ""
	}
	options := application.LogsQueryOptions{Tail: tail, All: data.All, Session: strings.TrimSpace(data.Session), Since: strings.TrimSpace(data.Since), Until: strings.TrimSpace(data.Until), Level: level, Components: strings.TrimSpace(data.Components), Workspace: strings.TrimSpace(data.Workspace), Tool: strings.TrimSpace(data.Tool), Status: strings.TrimSpace(data.Status), Source: strings.TrimSpace(data.Source), Event: strings.TrimSpace(data.Event), Grep: strings.TrimSpace(data.Grep)}
	if _, err := application.BuildLogsQuery(options, time.Now()); err != nil {
		return application.LogsQueryOptions{}, logger.VisibilityDefault, err
	}
	return options, visibility, nil
}

func logsVisibilityValue(visibility logger.Visibility) string {
	switch visibility {
	case logger.VisibilityDebug:
		return "debug"
	case logger.VisibilityVerbose:
		return "verbose"
	default:
		return "normal"
	}
}

func parseLogsVisibility(value string) (logger.Visibility, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "normal":
		return logger.VisibilityDefault, nil
	case "verbose":
		return logger.VisibilityVerbose, nil
	case "debug":
		return logger.VisibilityDebug, nil
	default:
		return logger.VisibilityDefault, fmt.Errorf("visibility must be normal, verbose, or debug")
	}
}
