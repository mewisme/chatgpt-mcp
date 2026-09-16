package cftunnel

import (
	"context"
	"encoding/json"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

type Handler struct {
	Manager *Manager
}

func NewHandler() *Handler {
	return &Handler{Manager: NewManager()}
}

func (h *Handler) Describe(context.Context, json.RawMessage) (any, error) {
	return runtimeplugin.DescribeResult{
		Provider: "cf",
		Name:     "CF Tunnel",
		Targets: []runtimeplugin.Target{
			{ID: TargetMCP, OriginKind: "mcp-http", RequiresAuthenticatedOrigin: true},
			{ID: TargetAdmin, OriginKind: "admin-http", RequiresAuthenticatedOrigin: true},
		},
	}, nil
}

func (h *Handler) Status(context.Context, json.RawMessage) (any, error) {
	return h.statusResult(), nil
}

func (h *Handler) Start(ctx context.Context, params json.RawMessage) (any, error) {
	var start runtimeplugin.StartParams
	if err := json.Unmarshal(params, &start); err != nil {
		return nil, err
	}
	if err := h.manager().ApplyStart(ctx, start.Target, start.Origin); err != nil {
		return nil, err
	}
	return h.statusResult(), nil
}

func (h *Handler) Stop(_ context.Context, params json.RawMessage) (any, error) {
	var stop runtimeplugin.StopParams
	if err := json.Unmarshal(params, &stop); err != nil {
		return nil, err
	}
	if err := h.manager().ApplyStop(stop.Target); err != nil {
		return nil, err
	}
	return h.statusResult(), nil
}

func (h *Handler) Shutdown(context.Context, json.RawMessage) (any, error) {
	h.manager().Stop()
	return map[string]bool{"ok": true}, nil
}

func (h *Handler) manager() *Manager {
	if h != nil && h.Manager != nil {
		return h.Manager
	}
	return live
}

func (h *Handler) statusResult() runtimeplugin.StatusResult {
	st := h.manager().Status()
	targets := make([]runtimeplugin.TargetStatus, 0, len(st.Targets))
	for _, item := range st.Targets {
		targets = append(targets, runtimeplugin.TargetStatus{
			Target: item.Target, Desired: item.Desired, Running: item.Running, Ready: item.Ready, Restarting: item.Restarting,
			URL: item.URL, Origin: item.Origin, LastError: item.LastError, Ephemeral: item.URL != "",
		})
	}
	state := "stopped"
	if st.Enabled {
		state = "running"
	}
	return runtimeplugin.StatusResult{State: state, Targets: targets}
}
