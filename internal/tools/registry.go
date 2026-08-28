package tools

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrToolNotFound          = errors.New("tool not found")
	ErrToolAlreadyRegistered = errors.New("tool already registered")
)

type Handler func(context.Context, map[string]any) (Result, error)

type Entry struct {
	Schema  Schema
	Handler Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Entry
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Entry{}} }

func (r *Registry) Register(name string, schema Schema, handler Handler) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("tool name is required")
	}
	if handler == nil {
		return fmt.Errorf("tool %q handler is required", name)
	}
	if schema.Name == "" {
		schema.Name = name
	}
	if schema.Name != name {
		return fmt.Errorf("tool schema name %q does not match registration name %q", schema.Name, name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %s", ErrToolAlreadyRegistered, name)
	}
	r.tools[name] = Entry{Schema: schema, Handler: handler}
	return nil
}

func (r *Registry) MustRegister(name string, schema Schema, handler Handler) {
	if err := r.Register(name, schema, handler); err != nil {
		panic(err)
	}
}

func (r *Registry) ListSchemas() []Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Schema, 0, len(r.tools))
	for _, entry := range r.tools {
		out = append(out, entry.Schema)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	r.mu.RLock()
	entry, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if args == nil {
		args = map[string]any{}
	}
	return entry.Handler(ctx, args)
}
