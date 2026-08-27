package mcp

import "sync"

type Registry struct {
	mu    sync.RWMutex
	tools map[string]any
}

func NewRegistry() *Registry { return &Registry{tools: map[string]any{}} }
func (r *Registry) Register(name string, tool any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = tool
}
func (r *Registry) List() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]any{}
	for k, v := range r.tools {
		out[k] = v
	}
	return out
}
