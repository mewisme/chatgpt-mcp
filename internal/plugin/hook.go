package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/controlplane"
)

const (
	HookSchema                                  = 1
	CapabilityHookPreToolUse      Capability    = "hook/pre-tool-use"
	CapabilityHookPostToolUse     Capability    = "hook/post-tool-use"
	CapabilityHookToolError       Capability    = "hook/tool-error"
	CapabilityHookToolDenied      Capability    = "hook/tool-denied"
	HookEventPreToolUse           HookEventType = "pre_tool_use"
	HookEventPostToolUse          HookEventType = "post_tool_use"
	HookEventToolError            HookEventType = "tool_error"
	HookEventToolDenied           HookEventType = "tool_denied"
	HookDecisionContinue          HookDecision  = "continue"
	HookDecisionDeny              HookDecision  = "deny"
	HookDecisionRequireApproval   HookDecision  = "require_approval"
	HookOriginAgent               HookOrigin    = "agent"
	HookOriginUser                HookOrigin    = "user"
	HookOriginHook                HookOrigin    = "hook"
	HookOriginSystem              HookOrigin    = "system"
	defaultPreHookTimeout                       = 5 * time.Second
	defaultObservationHookTimeout               = 5 * time.Second
	defaultObservationQueueSize                 = 128
	maxHookPayloadBytes                         = 256 << 10
	maxHookOutputBytes                          = 64 << 10
)

type HookEventType string
type HookDecision string
type HookOrigin string

type HookProvenance struct {
	ExecutionID       string     `json:"execution_id"`
	ParentExecutionID string     `json:"parent_execution_id,omitempty"`
	Origin            HookOrigin `json:"origin"`
	HookDepth         int        `json:"hook_depth"`
}

type HookResultMetadata struct {
	IsError      bool   `json:"is_error"`
	ResultType   string `json:"result_type,omitempty"`
	ContentCount int    `json:"content_count,omitempty"`
}

type HookEvent struct {
	Schema             int                 `json:"schema"`
	Type               HookEventType       `json:"type"`
	Provenance         HookProvenance      `json:"provenance"`
	Tool               string              `json:"tool"`
	WorkspaceID        string              `json:"workspace_id,omitempty"`
	CWD                string              `json:"cwd,omitempty"`
	RequestedArguments map[string]any      `json:"requested_arguments"`
	EffectiveArguments map[string]any      `json:"effective_arguments"`
	SecurityCommand    string              `json:"security_command,omitempty"`
	Result             *HookResultMetadata `json:"result,omitempty"`
	DurationMS         int64               `json:"duration_ms,omitempty"`
	Status             string              `json:"status,omitempty"`
	Error              string              `json:"error,omitempty"`
}

type HookProviderMetadata struct {
	PluginID   PluginID   `json:"plugin_id"`
	Version    Version    `json:"version"`
	Capability Capability `json:"capability"`
}

type HookResult struct {
	Schema    int                    `json:"schema"`
	Decision  HookDecision           `json:"decision"`
	Reason    string                 `json:"reason,omitempty"`
	Providers []HookProviderMetadata `json:"-"`
}

type HookStats struct {
	Dropped uint64 `json:"dropped"`
	Failed  uint64 `json:"failed"`
}

type HookRunner func(context.Context, CapabilityProvider, HookEvent) (HookResult, error)

type HookDispatcher struct {
	store              *Store
	preTimeout         time.Duration
	observationTimeout time.Duration
	queue              chan HookEvent
	runner             HookRunner
	mu                 sync.Mutex
	draining           bool
	dropped            atomic.Uint64
	failed             atomic.Uint64
}

type hookProvenanceContextKey struct{}

func WithHookProvenance(ctx context.Context, provenance HookProvenance) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	provenance.ExecutionID = strings.TrimSpace(provenance.ExecutionID)
	provenance.ParentExecutionID = strings.TrimSpace(provenance.ParentExecutionID)
	if provenance.HookDepth < 0 {
		provenance.HookDepth = 0
	}
	return context.WithValue(ctx, hookProvenanceContextKey{}, provenance)
}

func HookProvenanceFromContext(ctx context.Context) (HookProvenance, bool) {
	if ctx == nil {
		return HookProvenance{}, false
	}
	value, ok := ctx.Value(hookProvenanceContextKey{}).(HookProvenance)
	if !ok {
		return HookProvenance{}, false
	}
	return value, true
}

func NewHookDispatcher(store *Store) *HookDispatcher {
	return NewHookDispatcherWithRunner(store, defaultHookRunner)
}

func NewHookDispatcherWithRunner(store *Store, runner HookRunner) *HookDispatcher {
	if runner == nil {
		runner = defaultHookRunner
	}
	return &HookDispatcher{store: store, preTimeout: defaultPreHookTimeout, observationTimeout: defaultObservationHookTimeout, queue: make(chan HookEvent, defaultObservationQueueSize), runner: runner}
}

func (dispatcher *HookDispatcher) PreToolUse(ctx context.Context, event HookEvent) (HookResult, error) {
	if dispatcher == nil || dispatcher.store == nil || !shouldDispatchHooks(event) {
		return HookResult{Schema: HookSchema, Decision: HookDecisionContinue}, nil
	}
	event = normalizeHookEvent(event, HookEventPreToolUse)
	providers, err := dispatcher.providers(CapabilityHookPreToolUse)
	if err != nil {
		return HookResult{}, err
	}
	decision := HookResult{Schema: HookSchema, Decision: HookDecisionContinue}
	for _, provider := range providers {
		metadata := HookProviderMetadata{PluginID: provider.PluginID, Version: provider.Version, Capability: CapabilityHookPreToolUse}
		if !providerHasPermission(provider, PermissionProcessExecute) || !providerHasPermission(provider, PermissionHookToolControl) {
			decision.Providers = append(decision.Providers, metadata)
			return decision, fmt.Errorf("hook provider %s@%s lacks required control permissions", provider.PluginID, provider.Version)
		}
		runCtx, cancel := context.WithTimeout(ctx, dispatcher.preTimeout)
		result, runErr := dispatcher.runner(runCtx, provider, event)
		cancel()
		decision.Providers = append(decision.Providers, metadata)
		if runErr != nil {
			return decision, fmt.Errorf("pre-tool hook %s@%s failed: %w", provider.PluginID, provider.Version, runErr)
		}
		if err := result.Validate(); err != nil {
			return decision, fmt.Errorf("pre-tool hook %s@%s returned invalid result: %w", provider.PluginID, provider.Version, err)
		}
		switch result.Decision {
		case HookDecisionDeny:
			result.Providers = append([]HookProviderMetadata(nil), decision.Providers...)
			return result, nil
		case HookDecisionRequireApproval:
			if decision.Decision == HookDecisionContinue {
				decision.Decision = result.Decision
				decision.Reason = result.Reason
			}
		}
	}
	return decision, nil
}

func (dispatcher *HookDispatcher) Observe(event HookEvent) bool {
	if dispatcher == nil || dispatcher.store == nil || !isObservationEvent(event.Type) || !shouldDispatchHooks(event) {
		return false
	}
	event = normalizeHookEvent(event, event.Type)
	select {
	case dispatcher.queue <- event:
	default:
		dispatcher.dropped.Add(1)
		return false
	}
	dispatcher.startDrain()
	return true
}

func (dispatcher *HookDispatcher) Stats() HookStats {
	if dispatcher == nil {
		return HookStats{}
	}
	return HookStats{Dropped: dispatcher.dropped.Load(), Failed: dispatcher.failed.Load()}
}

func (dispatcher *HookDispatcher) WaitIdle(ctx context.Context) error {
	if dispatcher == nil {
		return nil
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		dispatcher.mu.Lock()
		idle := !dispatcher.draining && len(dispatcher.queue) == 0
		dispatcher.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (result HookResult) Validate() error {
	if result.Schema != HookSchema {
		return fmt.Errorf("unsupported hook result schema: %d", result.Schema)
	}
	switch result.Decision {
	case HookDecisionContinue:
		return nil
	case HookDecisionDeny, HookDecisionRequireApproval:
		if strings.TrimSpace(result.Reason) == "" {
			return errors.New("hook decision reason is required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported hook decision: %q", result.Decision)
	}
}

func (dispatcher *HookDispatcher) startDrain() {
	dispatcher.mu.Lock()
	if dispatcher.draining {
		dispatcher.mu.Unlock()
		return
	}
	dispatcher.draining = true
	dispatcher.mu.Unlock()
	go dispatcher.drain()
}

func (dispatcher *HookDispatcher) drain() {
	for {
		select {
		case event := <-dispatcher.queue:
			dispatcher.dispatchObservation(event)
		default:
			dispatcher.mu.Lock()
			if len(dispatcher.queue) == 0 {
				dispatcher.draining = false
				dispatcher.mu.Unlock()
				return
			}
			dispatcher.mu.Unlock()
		}
	}
}

func (dispatcher *HookDispatcher) dispatchObservation(event HookEvent) {
	capability, ok := capabilityForHookEvent(event.Type)
	if !ok {
		return
	}
	providers, err := dispatcher.providers(capability)
	if err != nil {
		dispatcher.failed.Add(1)
		return
	}
	for _, provider := range providers {
		if !providerHasPermission(provider, PermissionProcessExecute) || !providerHasPermission(provider, PermissionHookToolObserve) {
			dispatcher.failed.Add(1)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), dispatcher.observationTimeout)
		result, runErr := dispatcher.runner(ctx, provider, event)
		cancel()
		if runErr != nil || result.Validate() != nil || result.Decision != HookDecisionContinue {
			dispatcher.failed.Add(1)
		}
	}
}

func (dispatcher *HookDispatcher) providers(capability Capability) ([]CapabilityProvider, error) {
	resolver, err := NewResolver(dispatcher.store)
	if err != nil {
		return nil, err
	}
	return resolver.Providers(capability), nil
}

func capabilityForHookEvent(event HookEventType) (Capability, bool) {
	switch event {
	case HookEventPreToolUse:
		return CapabilityHookPreToolUse, true
	case HookEventPostToolUse:
		return CapabilityHookPostToolUse, true
	case HookEventToolError:
		return CapabilityHookToolError, true
	case HookEventToolDenied:
		return CapabilityHookToolDenied, true
	default:
		return "", false
	}
}

func normalizeHookEvent(event HookEvent, eventType HookEventType) HookEvent {
	event.Schema = HookSchema
	event.Type = eventType
	event.Tool = strings.TrimSpace(event.Tool)
	event.WorkspaceID = strings.TrimSpace(event.WorkspaceID)
	event.CWD = strings.TrimSpace(event.CWD)
	event.SecurityCommand = strings.TrimSpace(event.SecurityCommand)
	if event.RequestedArguments == nil {
		event.RequestedArguments = map[string]any{}
	}
	if event.EffectiveArguments == nil {
		event.EffectiveArguments = event.RequestedArguments
	}
	return event
}

func shouldDispatchHooks(event HookEvent) bool {
	return event.Provenance.Origin != HookOriginHook && event.Provenance.HookDepth == 0
}

func isObservationEvent(event HookEventType) bool {
	return event == HookEventPostToolUse || event == HookEventToolError || event == HookEventToolDenied
}

func providerHasPermission(provider CapabilityProvider, permission Permission) bool {
	for _, current := range provider.Permissions {
		if current == permission {
			return true
		}
	}
	return false
}

func defaultHookRunner(ctx context.Context, provider CapabilityProvider, event HookEvent) (HookResult, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return HookResult{}, fmt.Errorf("encode hook event: %w", err)
	}
	if len(payload) > maxHookPayloadBytes {
		return HookResult{}, fmt.Errorf("hook event exceeds %d-byte payload limit", maxHookPayloadBytes)
	}
	cmd := exec.CommandContext(ctx, provider.Path)
	cmd.Dir = filepath.Dir(provider.Path)
	cmd.Env = safeHookEnvironment()
	cmd.Stdin = bytes.NewReader(payload)
	stdout, stderr := &boundedHookBuffer{}, &boundedHookBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return HookResult{}, errors.New(message)
	}
	if stdout.exceeded {
		return HookResult{}, fmt.Errorf("hook output exceeds %d-byte limit", maxHookOutputBytes)
	}
	var result HookResult
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return HookResult{}, fmt.Errorf("decode hook result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return HookResult{}, errors.New("hook result contains multiple JSON values")
		}
		return HookResult{}, fmt.Errorf("decode hook result: %w", err)
	}
	return result, nil
}

type boundedHookBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (buffer *boundedHookBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := maxHookOutputBytes + 1 - buffer.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = buffer.buffer.Write(data)
	}
	if buffer.buffer.Len() > maxHookOutputBytes || len(data) < original {
		buffer.exceeded = true
	}
	return original, nil
}

func (buffer *boundedHookBuffer) String() string { return buffer.buffer.String() }

func safeHookEnvironment() []string {
	allowed := map[string]struct{}{"home": {}, "lang": {}, "path": {}, "systemroot": {}, "temp": {}, "tmp": {}, "userprofile": {}, "windir": {}}
	result := make([]string, 0, len(allowed)+1)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[strings.ToLower(key)]; keep {
			result = append(result, entry)
		}
	}
	sort.Strings(result)
	result = append(result, controlplane.ToolContextEnv+"=1")
	return result
}
