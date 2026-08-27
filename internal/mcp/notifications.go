package mcp

import "sync"

type Notifications struct {
	mu        sync.RWMutex
	listeners []chan string
}

func NewNotifications() *Notifications { return &Notifications{} }

func (n *Notifications) Subscribe() chan string {
	ch := make(chan string, 8)
	n.mu.Lock()
	n.listeners = append(n.listeners, ch)
	n.mu.Unlock()
	return ch
}

func (n *Notifications) ToolsChanged() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, ch := range n.listeners {
		select {
		case ch <- "notifications/tools/list_changed":
		default:
		}
	}
}
