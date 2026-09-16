package notification

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/state"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const (
	OpenActionAuto     = "auto"
	OpenActionDisabled = "disabled"
	dedupeLimit        = 256
	sendTimeout        = 5 * time.Second
)

type Settings struct {
	Enabled         bool
	Approvals       bool
	WhenTUIInactive bool
	OpenAction      string
}

func DefaultSettings() Settings {
	return Settings{Enabled: true, Approvals: true, WhenTUIInactive: true, OpenAction: OpenActionAuto}
}

type Options struct {
	Settings   func() Settings
	Provider   Provider
	Presence   func() (bool, error)
	Label      func(workspaceID, title, tool string) (workspaceLabel, action string)
	Log        *logger.Logger
	Workspaces *workspace.Manager
}

type Service struct {
	settings func() Settings
	provider Provider
	presence func() (bool, error)
	label    func(workspaceID, title, tool string) (string, string)
	log      *logger.Logger

	mu    sync.Mutex
	seen  map[string]struct{}
	order []string

	cancel context.CancelFunc
	stream *approval.EventStream
	sub    *approval.EventSubscription
}

func New(opts Options) *Service {
	if opts.Settings == nil {
		opts.Settings = DefaultSettings
	}
	if opts.Provider == nil {
		opts.Provider = PlatformProvider()
	}
	if opts.Presence == nil {
		opts.Presence = state.TUIReviewerActive
	}
	if opts.Label == nil {
		opts.Label = defaultLabels(opts.Workspaces)
	}
	return &Service{
		settings: opts.Settings,
		provider: opts.Provider,
		presence: opts.Presence,
		label:    opts.Label,
		log:      opts.Log,
		seen:     map[string]struct{}{},
	}
}

func (s *Service) Start(stream *approval.EventStream) {
	if s == nil || stream == nil {
		return
	}
	s.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.stream = stream
	s.sub = stream.Subscribe()
	go s.loop(ctx, s.sub)
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.stream != nil && s.sub != nil {
		s.stream.Unsubscribe(s.sub)
	}
	s.stream = nil
	s.sub = nil
}

func (s *Service) loop(ctx context.Context, sub *approval.EventSubscription) {
	if sub == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sub.Overflow:
			if !ok {
				return
			}
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			s.handle(event)
		}
	}
}

func (s *Service) handle(event approval.Event) {
	if event.Name != approval.EventRequested || event.Status != approval.StatusPending {
		return
	}
	id := strings.TrimSpace(event.RequestID)
	if id == "" || s.alreadySeen(id) {
		return
	}
	settings := s.settings()
	if !settings.Enabled || !settings.Approvals {
		s.note("notification.approval.suppressed", "Desktop approval notification suppressed", logger.With("reason", "disabled"), logger.With("request", id))
		return
	}
	if settings.WhenTUIInactive {
		active, err := s.presence()
		if err != nil {
			s.warn("notification.approval.presence-failed", "TUI presence check failed; delivering desktop notification", err, logger.With("request", id))
		} else if active {
			s.note("notification.approval.suppressed", "Desktop approval notification suppressed", logger.With("reason", "tui-active"), logger.With("request", id))
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	if !s.provider.Available(ctx) {
		cancel()
		s.note("notification.approval.suppressed", "Desktop approval notification suppressed", logger.With("reason", "unavailable"), logger.With("request", id))
		return
	}
	go func() {
		defer cancel()
		s.deliver(ctx, event, settings)
	}()
}

func (s *Service) deliver(ctx context.Context, event approval.Event, settings Settings) {
	caps := s.provider.Capabilities(ctx)
	openAction := settings.OpenAction != OpenActionDisabled && caps.Actions
	workspaceLabel, action := s.label(event.WorkspaceID, event.Title, event.TargetTool)
	note := Notification{
		RequestID:  event.RequestID,
		Title:      "ChatGPT MCP approval requested",
		Body:       workspaceLabel + " · " + action + "\nOpen CGM to review",
		OpenAction: openAction,
	}
	if err := s.provider.Send(ctx, note); err != nil {
		s.warn("notification.approval.failed", "Desktop approval notification failed", err, logger.With("request", event.RequestID), logger.With("workspace", event.WorkspaceID))
		return
	}
	s.note("notification.approval.sent", "Desktop approval notification sent", logger.With("request", event.RequestID), logger.With("workspace", event.WorkspaceID), logger.With("actions", note.OpenAction))
}

func (s *Service) alreadySeen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[id]; ok {
		return true
	}
	if len(s.order) >= dedupeLimit {
		old := s.order[0]
		s.order = append([]string(nil), s.order[1:]...)
		delete(s.seen, old)
	}
	s.seen[id] = struct{}{}
	s.order = append(s.order, id)
	return false
}

func (s *Service) note(name, message string, fields ...logger.Field) {
	if s.log != nil {
		s.log.Verbose("NOTIFICATION", name, message, fields...)
	}
}

func (s *Service) warn(name, message string, err error, fields ...logger.Field) {
	if s.log != nil {
		s.log.Warning("NOTIFICATION", name, message, err, fields...)
	}
}

func defaultLabels(workspaces *workspace.Manager) func(workspaceID, title, tool string) (string, string) {
	return func(workspaceID, title, _ string) (string, string) {
		action := strings.TrimSpace(title)
		if action == "" {
			action = "Approval request"
		}
		if workspaces != nil {
			if item, err := workspaces.Get(workspaceID); err == nil && item.Available() {
				if base := filepath.Base(item.Path); base != "" && base != "." && base != string(filepath.Separator) {
					return base, action
				}
			}
		}
		return "Workspace request", action
	}
}
