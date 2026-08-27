package upstream

import "sync"

type ToolCache struct {
	mu    sync.RWMutex
	items map[string][]Tool
}

func NewToolCache() *ToolCache { return &ToolCache{items: map[string][]Tool{}} }

func (c *ToolCache) Get(key string) ([]Tool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	return v, ok
}

func (c *ToolCache) Set(key string, tools []Tool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = tools
}
