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
	mu     sync.RWMutex
	tools  map[string]Entry
	owners map[string]string
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Entry{}, owners: map[string]string{}}
}

func (r *Registry) Register(name string, schema Schema, handler Handler) error {
	return r.registerOwned("", name, schema, handler)
}

func (r *Registry) registerOwned(owner, name string, schema Schema, handler Handler) error {
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
	if owner != "" {
		r.owners[name] = owner
	}
	return nil
}

func (r *Registry) MustRegister(name string, schema Schema, handler Handler) {
	if err := r.Register(name, schema, handler); err != nil {
		panic(err)
	}
}

func (r *Registry) ReplaceOwned(owner string, entries map[string]Entry) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return errors.New("owner is required")
	}
	for name, entry := range entries {
		if strings.TrimSpace(name) == "" || entry.Handler == nil {
			return fmt.Errorf("invalid owned tool %q", name)
		}
		if entry.Schema.Name == "" {
			entry.Schema.Name = name
			entries[name] = entry
		}
		if entry.Schema.Name != name {
			return fmt.Errorf("owned schema name %q does not match %q", entry.Schema.Name, name)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for name := range entries {
		if _, exists := r.tools[name]; exists && r.owners[name] != owner {
			return fmt.Errorf("%w: %s", ErrToolAlreadyRegistered, name)
		}
	}
	for name, currentOwner := range r.owners {
		if currentOwner == owner {
			delete(r.tools, name)
			delete(r.owners, name)
		}
	}
	for name, entry := range entries {
		r.tools[name] = entry
		r.owners[name] = owner
	}
	return nil
}

func (r *Registry) ClearOwnedPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, owner := range r.owners {
		if strings.HasPrefix(owner, prefix) {
			delete(r.tools, name)
			delete(r.owners, name)
		}
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
