package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	defaultSubscriptionKeepAlive = 15 * time.Second
	defaultMaxSubscriptions      = 1024
)

type subscriptionHub struct {
	mu      sync.Mutex
	active  int
	closed  bool
	closeCh chan struct{}
}

func newSubscriptionHub() *subscriptionHub {
	return &subscriptionHub{closeCh: make(chan struct{})}
}

func (h *subscriptionHub) acquire() (<-chan struct{}, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errors.New("subscription service is closed")
	}
	if h.active >= defaultMaxSubscriptions {
		return nil, errors.New("too many active subscriptions")
	}
	h.active++
	return h.closeCh, nil
}

func (h *subscriptionHub) release() {
	h.mu.Lock()
	if h.active > 0 {
		h.active--
	}
	h.mu.Unlock()
}

func (h *subscriptionHub) closeAll() {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		close(h.closeCh)
	}
	h.mu.Unlock()
}

func (h HTTPRuntime) serveSubscription(w http.ResponseWriter, r *http.Request, req Request, params map[string]any) {
	if h.Subscriptions == nil || h.Server == nil || h.Server.Tools == nil || h.Server.Tools.Registry == nil {
		writeErrorID(w, req.ID, ErrInternal, "subscription runtime unavailable")
		return
	}
	closeCh, err := h.Subscriptions.acquire()
	if err != nil {
		writeErrorID(w, req.ID, ErrInternal, err.Error())
		return
	}
	defer h.Subscriptions.release()

	notifications, _ := params["notifications"].(map[string]any)
	toolsRequested, _ := notifications["toolsListChanged"].(bool)
	honoredTools := toolsRequested && DefaultCapabilities().Tools.ListChanged

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	meta := map[string]any{"io.modelcontextprotocol/subscriptionId": req.ID}
	honored := map[string]any{}
	if honoredTools {
		honored["toolsListChanged"] = true
	}
	ack := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/subscriptions/acknowledged",
		"params":  map[string]any{"notifications": honored, "_meta": meta},
	}
	if err := writeSubscriptionFrame(w, ack); err != nil {
		return
	}
	flusher.Flush()

	var changes <-chan struct{}
	var changeSubscription chan struct{}
	if honoredTools {
		changeSubscription = h.Server.Tools.Registry.SubscribeChanges()
		defer h.Server.Tools.Registry.UnsubscribeChanges(changeSubscription)
		changes = changeSubscription
	}

	keepAlive := time.NewTicker(defaultSubscriptionKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-closeCh:
			result := map[string]any{
				"resultType": "complete",
				"_meta": map[string]any{
					"io.modelcontextprotocol/subscriptionId": req.ID,
					"io.modelcontextprotocol/serverInfo":     serverInfo(),
				},
			}
			if err := writeSubscriptionFrame(w, Response{JSONRPC: "2.0", ID: req.ID, Result: result}); err == nil {
				flusher.Flush()
			}
			return
		case <-changes:
			notification := map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/tools/list_changed",
				"params":  map[string]any{"_meta": meta},
			}
			if err := writeSubscriptionFrame(w, notification); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSubscriptionFrame(w http.ResponseWriter, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func validateListenNotifications(params map[string]any) error {
	raw, exists := params["notifications"]
	if !exists {
		return NewError(ErrInvalidParams, "notifications is required")
	}
	notifications, ok := raw.(map[string]any)
	if !ok {
		return NewError(ErrInvalidParams, "notifications must be an object")
	}
	for _, key := range []string{"toolsListChanged", "promptsListChanged", "resourcesListChanged"} {
		if value, exists := notifications[key]; exists {
			if _, ok := value.(bool); !ok {
				return NewError(ErrInvalidParams, key+" must be a boolean")
			}
		}
	}
	if value, exists := notifications["resourceSubscriptions"]; exists {
		items, ok := value.([]any)
		if !ok {
			return NewError(ErrInvalidParams, "resourceSubscriptions must be an array")
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return NewError(ErrInvalidParams, "resourceSubscriptions must contain strings")
			}
		}
	}
	return nil
}
