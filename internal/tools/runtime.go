package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/caveman"
	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/features"
	"go.mewis.me/chatgpt-mcp/internal/ponytail"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Runtime struct {
	Registry        *Registry
	Workspaces      *workspace.Manager
	Checkpoints     *checkpoint.Store
	Upstream        *upstream.Manager
	CallObserver    CallObserver
	clusterMu       sync.RWMutex
	clusterNode     *cluster.Node
	featureMu       sync.Mutex
	features        features.Config
	ponytailManager *ponytail.Manager
	cavemanManager  *caveman.Manager
}

func NewRuntime() *Runtime {
	return NewRuntimeWithFeatures(features.Default())
}

func NewRuntimeWithFeatures(featureConfig features.Config) *Runtime {
	return NewRuntimeWithAccess(featureConfig, nil)
}

func NewRuntimeWithAccess(featureConfig features.Config, globalAllowDirs []string) *Runtime {
	workspaces := workspace.NewManagerWithGlobalAllowDirs(workspace.DefaultStorePath(), globalAllowDirs)
	checkpoints := checkpoint.NewStore(checkpoint.DefaultRoot())
	upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
	_ = upstreams.Load()
	registry := NewRegistry()
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints, Upstream: upstreams, ponytailManager: ponytail.NewManager(), cavemanManager: caveman.NewManager()}
	shell := shellruntime.NewManager(workspaces, shellruntime.DefaultStateRoot())
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterCore(registry, workspaces, checkpoints, shell)
	RegisterClusterTools(registry, runtime)
	RegisterUpstreamTools(registry, upstreams)
	if err := runtime.SyncFeatures(featureConfig); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = RefreshUpstreamProxies(ctx, registry, upstreams, false)
	cancel()
	return runtime
}

func (r *Runtime) SyncFeatures(featureConfig features.Config) error {
	if r == nil || r.Registry == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	r.featureMu.Lock()
	defer r.featureMu.Unlock()
	if r.ponytailManager == nil {
		r.ponytailManager = ponytail.NewManager()
	}
	if r.cavemanManager == nil {
		r.cavemanManager = caveman.NewManager()
	}
	if err := r.Registry.ReplaceOwnedPrefix("feature:", featureToolEntries(r.Workspaces, r.ponytailManager, r.cavemanManager, featureConfig)); err != nil {
		return err
	}
	r.features = featureConfig
	return nil
}

func (r *Runtime) Features() features.Config {
	if r == nil {
		return features.Config{}
	}
	r.featureMu.Lock()
	defer r.featureMu.Unlock()
	return r.features
}

func (r *Runtime) SetGlobalAllowDirs(allowDirs []string) {
	if r != nil && r.Workspaces != nil {
		r.Workspaces.SetGlobalAllowDirs(allowDirs)
	}
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }

func (r *Runtime) SetClusterNode(node *cluster.Node) {
	if r == nil {
		return
	}
	r.clusterMu.Lock()
	r.clusterNode = node
	r.clusterMu.Unlock()
	if node != nil {
		node.SetAdvertisementProvider(r.ClusterAdvertisement)
	}
}

func (r *Runtime) ClusterNode() *cluster.Node {
	if r == nil {
		return nil
	}
	r.clusterMu.RLock()
	defer r.clusterMu.RUnlock()
	return r.clusterNode
}

func (r *Runtime) ClusterAdvertisement() (cluster.Advertisement, error) {
	if r == nil || r.Workspaces == nil {
		return cluster.Advertisement{}, errors.New("workspace manager is unavailable")
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return cluster.Advertisement{}, err
	}
	workspaceIDs, err := r.Workspaces.AdvertisedIDs()
	if err != nil {
		return cluster.Advertisement{}, err
	}
	catalogHash, err := CatalogHash(r.List())
	if err != nil {
		return cluster.Advertisement{}, err
	}
	return cluster.Advertisement{InstanceID: identity.ID, Name: identity.Name, CatalogHash: catalogHash, Workspaces: workspaceIDs}, nil
}

func (r *Runtime) RefreshClusterAdvertisement(ctx context.Context) error {
	node := r.ClusterNode()
	if node == nil {
		return nil
	}
	value, err := r.ClusterAdvertisement()
	if err != nil {
		return err
	}
	return node.Update(ctx, value)
}

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	return r.call(ctx, name, args, true)
}

func (r *Runtime) call(ctx context.Context, name string, args map[string]any, allowRemote bool) (Result, error) {
	started := time.Now()
	source := CallSource(ctx)
	workspaceID, _ := args["workspace_id"].(string)
	if workspaceID != "" && r.Workspaces != nil {
		if canonical, err := r.Workspaces.CanonicalID(workspaceID); err == nil && canonical != workspaceID {
			args = cloneMap(args)
			args["workspace_id"] = canonical
			workspaceID = canonical
		}
	}
	raw := callRaw(ctx, source, name, args)
	r.observeCall(CallObservation{Phase: "start", Source: source, Tool: name, WorkspaceID: workspaceID, Raw: raw})

	result, err := r.executeCall(ctx, name, args, workspaceID, allowRemote)
	if err == nil && !result.IsError && name == "workspace_register" {
		_ = r.RefreshClusterAdvertisement(ctx)
	}
	if err == nil {
		if result.ResultType == "" {
			result.ResultType = "complete"
		}
		status, message := "ok", ""
		if result.IsError {
			status = "error"
			if len(result.Content) > 0 {
				message = result.Content[0].Text
			}
		}
		finishRaw := cloneMap(raw)
		finishRaw["status"] = status
		finishRaw["result_type"] = result.ResultType
		finishRaw["result"] = result
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw})
		return result, nil
	}

	status, message := "error", err.Error()
	if ctx != nil && ctx.Err() != nil {
		status, message = "cancelled", ctx.Err().Error()
	}
	if errors.Is(err, ErrToolNotFound) {
		finishRaw := cloneMap(raw)
		finishRaw["status"] = status
		finishRaw["error"] = message
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, Raw: finishRaw})
		return Result{}, err
	}
	result = ErrorResult(err)
	finishRaw := cloneMap(raw)
	finishRaw["status"] = status
	finishRaw["result_type"] = result.ResultType
	finishRaw["result"] = result
	finishRaw["error"] = message
	r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw})
	return result, nil
}

const (
	clusterToolCallMethod          = "tools.call"
	clusterWorkspaceRegisterMethod = "workspace.register"
	clusterWorkspaceListMethod     = "workspace.list"
)

type clusterToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type clusterToolResponse struct {
	Result       Result `json:"result"`
	Error        string `json:"error,omitempty"`
	ToolNotFound bool   `json:"tool_not_found,omitempty"`
}

func (r *Runtime) executeCall(ctx context.Context, name string, args map[string]any, workspaceID string, allowRemote bool) (Result, error) {
	if allowRemote && name == "workspace_register" {
		return r.executeWorkspaceRegister(ctx, args)
	}
	if !allowRemote || workspaceID == "" || r.Workspaces == nil {
		return r.Registry.Call(ctx, name, args)
	}
	if _, err := r.Workspaces.Get(workspaceID); err == nil {
		return r.Registry.Call(ctx, name, args)
	}
	node := r.ClusterNode()
	if node == nil {
		return r.Registry.Call(ctx, name, args)
	}
	owner, err := node.WorkspaceOwner(ctx, workspaceID)
	if err != nil {
		return Result{}, err
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return Result{}, err
	}
	if owner.InstanceID == identity.ID {
		return Result{}, fmt.Errorf("cluster directory maps workspace %s to this instance but it is not registered locally", workspaceID)
	}
	payload, err := json.Marshal(clusterToolCall{Name: name, Arguments: args})
	if err != nil {
		return Result{}, err
	}
	encoded, err := node.Call(ctx, owner.InstanceID, clusterToolCallMethod, payload)
	if err != nil {
		return Result{}, err
	}
	var response clusterToolResponse
	if err := json.Unmarshal(encoded, &response); err != nil {
		return Result{}, fmt.Errorf("decode cluster tool response: %w", err)
	}
	if response.Error != "" {
		if response.ToolNotFound {
			return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, response.Error)
		}
		return Result{}, errors.New(response.Error)
	}
	return response.Result, nil
}

func (r *Runtime) executeWorkspaceRegister(ctx context.Context, args map[string]any) (Result, error) {
	if r.Workspaces == nil {
		return Result{}, errors.New("workspace manager is unavailable")
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return Result{}, err
	}
	target, err := optionalString(args, "instance_id")
	if err != nil {
		return Result{}, err
	}
	localArgs := cloneMap(args)
	delete(localArgs, "instance_id")
	if target == "" || target == identity.ID {
		return r.Registry.Call(ctx, "workspace_register", localArgs)
	}
	node := r.ClusterNode()
	if node == nil {
		return Result{}, fmt.Errorf("cluster instance is unavailable: %s", target)
	}
	if _, err := clusterMember(ctx, node, target); err != nil {
		return Result{}, err
	}
	payload, err := json.Marshal(clusterToolCall{Name: "workspace_register", Arguments: localArgs})
	if err != nil {
		return Result{}, err
	}
	encoded, err := node.Call(ctx, target, clusterWorkspaceRegisterMethod, payload)
	if err != nil {
		return Result{}, err
	}
	return decodeClusterToolResponse(encoded)
}

func (r *Runtime) ClusterRPCHandler(ctx context.Context, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch method {
	case clusterWorkspaceRegisterMethod:
		var call clusterToolCall
		if err := json.Unmarshal(payload, &call); err != nil {
			return nil, fmt.Errorf("decode cluster workspace register: %w", err)
		}
		if call.Name != "workspace_register" {
			return nil, errors.New("cluster workspace register payload has unexpected tool name")
		}
		call.Arguments = cloneMap(call.Arguments)
		delete(call.Arguments, "instance_id")
		ctx = WithCallSource(ctx, "cluster")
		result, callErr := r.call(ctx, call.Name, call.Arguments, false)
		return encodeClusterToolResponse(result, callErr)
	case clusterWorkspaceListMethod:
		value, err := r.localWorkspaceList()
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case clusterToolCallMethod:
	default:
		return nil, fmt.Errorf("unsupported cluster RPC method: %s", method)
	}
	var call clusterToolCall
	if err := json.Unmarshal(payload, &call); err != nil {
		return nil, fmt.Errorf("decode cluster tool call: %w", err)
	}
	workspaceID, _ := call.Arguments["workspace_id"].(string)
	if workspaceID == "" {
		return nil, errors.New("cluster tool call requires workspace_id")
	}
	if r.Workspaces == nil {
		return nil, errors.New("workspace manager is unavailable")
	}
	canonical, err := r.Workspaces.CanonicalID(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("cluster routed workspace is not owned by this instance: %s", workspaceID)
	}
	if canonical != workspaceID {
		call.Arguments = cloneMap(call.Arguments)
		call.Arguments["workspace_id"] = canonical
	}
	ctx = WithCallSource(ctx, "cluster")
	result, callErr := r.call(ctx, call.Name, call.Arguments, false)
	return encodeClusterToolResponse(result, callErr)
}

func encodeClusterToolResponse(result Result, err error) (json.RawMessage, error) {
	response := clusterToolResponse{Result: result}
	if err != nil {
		response.Error = err.Error()
		response.ToolNotFound = errors.Is(err, ErrToolNotFound)
	}
	return json.Marshal(response)
}

func decodeClusterToolResponse(encoded json.RawMessage) (Result, error) {
	var response clusterToolResponse
	if err := json.Unmarshal(encoded, &response); err != nil {
		return Result{}, fmt.Errorf("decode cluster tool response: %w", err)
	}
	if response.Error == "" {
		return response.Result, nil
	}
	if response.ToolNotFound {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, response.Error)
	}
	return Result{}, errors.New(response.Error)
}

func clusterMember(ctx context.Context, node *cluster.Node, instanceID string) (cluster.Member, error) {
	snapshot, err := node.Snapshot(ctx)
	if err != nil {
		return cluster.Member{}, err
	}
	for _, member := range snapshot.Members {
		if member.InstanceID != instanceID {
			continue
		}
		if !member.Online {
			return member, fmt.Errorf("cluster instance is offline: %s", instanceID)
		}
		return member, nil
	}
	return cluster.Member{}, fmt.Errorf("cluster instance not found: %s", instanceID)
}
