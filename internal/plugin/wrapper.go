package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	WrapperSchema           = 1
	commandWrapperPrefix    = "command-wrapper/"
	defaultWrapperTimeout   = 2 * time.Second
	maxWrapperPayloadBytes  = 64 << 10
	maxWrapperOutputBytes   = 16 << 10
	WrapperOperationCanWrap = "can_wrap"
	WrapperOperationRewrite = "rewrite"
	WrapperOperationProject = "security_projection"
)

type CommandWrapperRequest struct {
	Schema    int    `json:"schema"`
	Operation string `json:"operation"`
	Tool      string `json:"tool"`
	Command   string `json:"command"`
}

type CommandWrapperResponse struct {
	Schema  int    `json:"schema"`
	CanWrap *bool  `json:"can_wrap,omitempty"`
	Command string `json:"command,omitempty"`
}

type CommandWrapperInfo struct {
	Capability Capability `json:"capability"`
	PluginID   PluginID   `json:"plugin_id"`
	Version    Version    `json:"version"`
	Path       string     `json:"-"`
}

type CommandPlan struct {
	Requested string              `json:"requested"`
	Effective string              `json:"effective"`
	Security  string              `json:"security"`
	Wrapper   *CommandWrapperInfo `json:"wrapper,omitempty"`
}

type CommandWrapperRunner func(context.Context, CapabilityProvider, CommandWrapperRequest) (CommandWrapperResponse, error)

type CommandWrapperConflictError struct {
	Tool      string
	Command   string
	Providers []CommandWrapperInfo
}

func (err CommandWrapperConflictError) Error() string {
	providers := make([]string, 0, len(err.Providers))
	for _, provider := range err.Providers {
		providers = append(providers, fmt.Sprintf("%s:%s@%s", provider.Capability, provider.PluginID, provider.Version))
	}
	return fmt.Sprintf("multiple command wrappers apply to %s: %s", err.Tool, strings.Join(providers, ", "))
}

type CommandWrapperPipeline struct {
	store   *Store
	timeout time.Duration
	runner  CommandWrapperRunner
}

func NewCommandWrapperPipeline(store *Store) *CommandWrapperPipeline {
	return NewCommandWrapperPipelineWithRunner(store, defaultCommandWrapperRunner)
}

func NewCommandWrapperPipelineWithRunner(store *Store, runner CommandWrapperRunner) *CommandWrapperPipeline {
	if runner == nil {
		runner = defaultCommandWrapperRunner
	}
	return &CommandWrapperPipeline{store: store, timeout: defaultWrapperTimeout, runner: runner}
}

func (pipeline *CommandWrapperPipeline) Apply(ctx context.Context, tool, command string) (CommandPlan, error) {
	requested := strings.TrimSpace(command)
	plan := CommandPlan{Requested: requested, Effective: requested, Security: requested}
	if pipeline == nil || pipeline.store == nil || requested == "" {
		return plan, nil
	}
	resolver, err := NewResolver(pipeline.store)
	if err != nil {
		return CommandPlan{}, err
	}
	capabilities := resolver.Capabilities(commandWrapperPrefix)
	type applicableWrapper struct {
		capability Capability
		provider   CapabilityProvider
	}
	applicable := make([]applicableWrapper, 0, len(capabilities))
	for _, capability := range capabilities {
		provider, err := resolver.Resolve(capability)
		if err != nil {
			return CommandPlan{}, err
		}
		if !providerHasPermission(provider, PermissionProcessExecute) {
			return CommandPlan{}, fmt.Errorf("command wrapper %s@%s lacks required permission %s", provider.PluginID, provider.Version, PermissionProcessExecute)
		}
		name := strings.TrimPrefix(string(capability), commandWrapperPrefix)
		if executable := wrapperExecutableName(provider.Path); executable != name {
			return CommandPlan{}, fmt.Errorf("command wrapper %s@%s entrypoint %q does not match capability %q", provider.PluginID, provider.Version, executable, name)
		}
		response, err := pipeline.run(ctx, provider, CommandWrapperRequest{Schema: WrapperSchema, Operation: WrapperOperationCanWrap, Tool: strings.TrimSpace(tool), Command: requested})
		if err != nil {
			return CommandPlan{}, fmt.Errorf("command wrapper %s@%s can-wrap failed: %w", provider.PluginID, provider.Version, err)
		}
		if err := validateWrapperResponse(response, WrapperOperationCanWrap); err != nil {
			return CommandPlan{}, fmt.Errorf("command wrapper %s@%s can-wrap response: %w", provider.PluginID, provider.Version, err)
		}
		if response.CanWrap != nil && *response.CanWrap {
			applicable = append(applicable, applicableWrapper{capability: capability, provider: provider})
		}
	}
	if len(applicable) == 0 {
		return plan, nil
	}
	if len(applicable) > 1 {
		providers := make([]CommandWrapperInfo, 0, len(applicable))
		for _, item := range applicable {
			providers = append(providers, CommandWrapperInfo{Capability: item.capability, PluginID: item.provider.PluginID, Version: item.provider.Version})
		}
		return CommandPlan{}, CommandWrapperConflictError{Tool: strings.TrimSpace(tool), Command: requested, Providers: providers}
	}
	selected := applicable[0]
	name := strings.TrimPrefix(string(selected.capability), commandWrapperPrefix)
	rewrite, err := pipeline.run(ctx, selected.provider, CommandWrapperRequest{Schema: WrapperSchema, Operation: WrapperOperationRewrite, Tool: strings.TrimSpace(tool), Command: requested})
	if err != nil {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s rewrite failed: %w", selected.provider.PluginID, selected.provider.Version, err)
	}
	if err := validateWrapperResponse(rewrite, WrapperOperationRewrite); err != nil {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s rewrite response: %w", selected.provider.PluginID, selected.provider.Version, err)
	}
	effective := strings.TrimSpace(rewrite.Command)
	expected := name + " " + requested
	if effective != expected {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s rewrite is not transparent: expected %q, got %q", selected.provider.PluginID, selected.provider.Version, expected, effective)
	}
	projection, err := pipeline.run(ctx, selected.provider, CommandWrapperRequest{Schema: WrapperSchema, Operation: WrapperOperationProject, Tool: strings.TrimSpace(tool), Command: effective})
	if err != nil {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s security projection failed: %w", selected.provider.PluginID, selected.provider.Version, err)
	}
	if err := validateWrapperResponse(projection, WrapperOperationProject); err != nil {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s security projection response: %w", selected.provider.PluginID, selected.provider.Version, err)
	}
	security := strings.TrimSpace(projection.Command)
	if security != requested {
		return CommandPlan{}, fmt.Errorf("command wrapper %s@%s security projection must preserve the requested command: expected %q, got %q", selected.provider.PluginID, selected.provider.Version, requested, security)
	}
	plan.Effective = effective
	plan.Security = security
	plan.Wrapper = &CommandWrapperInfo{Capability: selected.capability, PluginID: selected.provider.PluginID, Version: selected.provider.Version, Path: selected.provider.Path}
	return plan, nil
}

func wrapperExecutableName(path string) string {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	return strings.TrimSuffix(name, ".exe")
}

func (pipeline *CommandWrapperPipeline) run(ctx context.Context, provider CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := pipeline.timeout
	if timeout <= 0 {
		timeout = defaultWrapperTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return pipeline.runner(runCtx, provider, request)
}

func validateWrapperResponse(response CommandWrapperResponse, operation string) error {
	if response.Schema != WrapperSchema {
		return fmt.Errorf("unsupported wrapper response schema: %d", response.Schema)
	}
	switch operation {
	case WrapperOperationCanWrap:
		if response.CanWrap == nil {
			return errors.New("can_wrap response is required")
		}
		if strings.TrimSpace(response.Command) != "" {
			return errors.New("can_wrap response must not include command")
		}
	case WrapperOperationRewrite, WrapperOperationProject:
		if response.CanWrap != nil {
			return errors.New("command response must not include can_wrap")
		}
		if strings.TrimSpace(response.Command) == "" {
			return errors.New("command response is required")
		}
	default:
		return fmt.Errorf("unsupported wrapper operation: %q", operation)
	}
	return nil
}

func defaultCommandWrapperRunner(ctx context.Context, provider CapabilityProvider, request CommandWrapperRequest) (CommandWrapperResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return CommandWrapperResponse{}, fmt.Errorf("encode command wrapper request: %w", err)
	}
	if len(payload) > maxWrapperPayloadBytes {
		return CommandWrapperResponse{}, fmt.Errorf("command wrapper request exceeds %d-byte payload limit", maxWrapperPayloadBytes)
	}
	cmd := exec.CommandContext(ctx, provider.Path)
	cmd.Dir = filepath.Dir(provider.Path)
	cmd.Env = safeHookEnvironment()
	cmd.Stdin = bytes.NewReader(payload)
	stdout, stderr := &boundedWrapperBuffer{}, &boundedWrapperBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return CommandWrapperResponse{}, errors.New(message)
	}
	if stdout.exceeded {
		return CommandWrapperResponse{}, fmt.Errorf("command wrapper output exceeds %d-byte limit", maxWrapperOutputBytes)
	}
	var response CommandWrapperResponse
	decoder := json.NewDecoder(strings.NewReader(stdout.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return CommandWrapperResponse{}, fmt.Errorf("decode command wrapper response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return CommandWrapperResponse{}, errors.New("command wrapper response contains multiple JSON values")
		}
		return CommandWrapperResponse{}, fmt.Errorf("decode command wrapper response: %w", err)
	}
	return response, nil
}

type boundedWrapperBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (buffer *boundedWrapperBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := maxWrapperOutputBytes + 1 - buffer.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = buffer.buffer.Write(data)
	}
	if buffer.buffer.Len() > maxWrapperOutputBytes || len(data) < original {
		buffer.exceeded = true
	}
	return original, nil
}

func (buffer *boundedWrapperBuffer) String() string { return buffer.buffer.String() }
