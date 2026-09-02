package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
)

type RuntimeListResult struct {
	Runtimes []cluster.Member `json:"runtimes"`
	Count    int              `json:"count"`
}

type WorkspaceListItem struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceRoot string `json:"workspace_root"`
	InstanceID    string `json:"instance_id"`
	InstanceName  string `json:"instance_name"`
	Online        bool   `json:"online"`
}

type WorkspaceListResult struct {
	Workspaces []WorkspaceListItem `json:"workspaces"`
	Count      int                 `json:"count"`
}

func RegisterClusterTools(registry *Registry, runtime *Runtime) {
	registry.MustRegister("runtime_list", Schema{
		Name:         "runtime_list",
		Title:        "List Runtimes",
		Description:  "List chatgpt-mcp runtime instances visible in the current cluster.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"runtimes":{"type":"array","items":{"type":"object","additionalProperties":true}},"count":{"type":"integer"}},"required":["runtimes","count"],"additionalProperties":false}`),
		Annotations:  ToolAnnotations(RiskRead),
	}, func(ctx context.Context, _ map[string]any) (Result, error) {
		value, err := runtime.runtimeList(ctx)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(value), nil
	})

	registry.MustRegister("workspace_list", Schema{
		Name:         "workspace_list",
		Title:        "List Workspaces",
		Description:  "List registered workspaces across online runtime instances, optionally limited to one instance_id.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"instance_id":{"type":"string"}},"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"workspaces":{"type":"array","items":{"type":"object","properties":{"workspace_id":{"type":"string"},"workspace_root":{"type":"string"},"instance_id":{"type":"string"},"instance_name":{"type":"string"},"online":{"type":"boolean"}},"required":["workspace_id","workspace_root","instance_id","instance_name","online"],"additionalProperties":false}},"count":{"type":"integer"}},"required":["workspaces","count"],"additionalProperties":false}`),
		Annotations:  ToolAnnotations(RiskRead),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		instanceID, err := optionalString(args, "instance_id")
		if err != nil {
			return Result{}, err
		}
		value, err := runtime.workspaceList(ctx, instanceID)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(value), nil
	})
}

func (r *Runtime) runtimeList(ctx context.Context) (RuntimeListResult, error) {
	if r == nil || r.Workspaces == nil {
		return RuntimeListResult{}, errors.New("workspace manager is unavailable")
	}
	node := r.ClusterNode()
	if node != nil {
		snapshot, err := node.Snapshot(ctx)
		if err != nil {
			return RuntimeListResult{}, err
		}
		return RuntimeListResult{Runtimes: snapshot.Members, Count: len(snapshot.Members)}, nil
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return RuntimeListResult{}, err
	}
	workspaceIDs, err := r.Workspaces.AdvertisedIDs()
	if err != nil {
		return RuntimeListResult{}, err
	}
	member := cluster.Member{InstanceID: identity.ID, Name: identity.Name, Workspaces: workspaceIDs, Online: true}
	return RuntimeListResult{Runtimes: []cluster.Member{member}, Count: 1}, nil
}

func (r *Runtime) workspaceList(ctx context.Context, instanceID string) (WorkspaceListResult, error) {
	if r == nil || r.Workspaces == nil {
		return WorkspaceListResult{}, errors.New("workspace manager is unavailable")
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return WorkspaceListResult{}, err
	}
	if instanceID == identity.ID || instanceID == "" && r.ClusterNode() == nil {
		return r.localWorkspaceList()
	}
	node := r.ClusterNode()
	if node == nil {
		return WorkspaceListResult{}, fmt.Errorf("cluster instance is unavailable: %s", instanceID)
	}
	if instanceID != "" {
		if _, err := clusterMember(ctx, node, instanceID); err != nil {
			return WorkspaceListResult{}, err
		}
		return r.remoteWorkspaceList(ctx, node, instanceID)
	}
	snapshot, err := node.Snapshot(ctx)
	if err != nil {
		return WorkspaceListResult{}, err
	}
	result := WorkspaceListResult{}
	for _, member := range snapshot.Members {
		if !member.Online {
			continue
		}
		var value WorkspaceListResult
		if member.InstanceID == identity.ID {
			value, err = r.localWorkspaceList()
		} else {
			value, err = r.remoteWorkspaceList(ctx, node, member.InstanceID)
		}
		if err != nil {
			return WorkspaceListResult{}, err
		}
		result.Workspaces = append(result.Workspaces, value.Workspaces...)
	}
	sortWorkspaceList(result.Workspaces)
	result.Count = len(result.Workspaces)
	return result, nil
}

func (r *Runtime) localWorkspaceList() (WorkspaceListResult, error) {
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return WorkspaceListResult{}, err
	}
	items, err := r.Workspaces.List()
	if err != nil {
		return WorkspaceListResult{}, err
	}
	result := WorkspaceListResult{Workspaces: make([]WorkspaceListItem, 0, len(items))}
	for _, item := range items {
		result.Workspaces = append(result.Workspaces, WorkspaceListItem{WorkspaceID: item.ID, WorkspaceRoot: item.Path, InstanceID: identity.ID, InstanceName: identity.Name, Online: true})
	}
	sortWorkspaceList(result.Workspaces)
	result.Count = len(result.Workspaces)
	return result, nil
}

func (r *Runtime) remoteWorkspaceList(ctx context.Context, node *cluster.Node, instanceID string) (WorkspaceListResult, error) {
	encoded, err := node.Call(ctx, instanceID, clusterWorkspaceListMethod, nil)
	if err != nil {
		return WorkspaceListResult{}, err
	}
	var value WorkspaceListResult
	if err := json.Unmarshal(encoded, &value); err != nil {
		return WorkspaceListResult{}, fmt.Errorf("decode cluster workspace list: %w", err)
	}
	sortWorkspaceList(value.Workspaces)
	value.Count = len(value.Workspaces)
	return value, nil
}

func sortWorkspaceList(values []WorkspaceListItem) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].InstanceID != values[j].InstanceID {
			return values[i].InstanceID < values[j].InstanceID
		}
		return values[i].WorkspaceID < values[j].WorkspaceID
	})
}
