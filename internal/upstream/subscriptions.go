package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var ErrSubscriptionsUnsupported = errors.New("upstream subscriptions/listen is not supported")

type subscriptionNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  map[string]any  `json:"params,omitempty"`
}

func (c *NativeClient) ListenToolsChanged(ctx context.Context, id string, onChange func()) error {
	if onChange == nil {
		return errors.New("tools changed callback is required")
	}
	connection, err := c.connection(id)
	if err != nil {
		return err
	}
	connection.mu.Lock()
	transport := connection.http
	era := connection.era
	supported := connection.toolsListChanged
	connection.mu.Unlock()
	if transport == nil || era != ModernProtocol || !supported {
		return ErrSubscriptionsUnsupported
	}

	requestID := connection.nextID.Add(1)
	request := rpcRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "subscriptions/listen",
		Params: map[string]any{
			"notifications": map[string]any{"toolsListChanged": true},
			"_meta":         requestMeta(ctx),
		},
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return transport.listen(ctx, data, requestID, func(message subscriptionNotification) {
		if message.Method == "notifications/tools/list_changed" {
			onChange()
		}
	})
}

func (t *httpTransport) listen(ctx context.Context, data []byte, requestID int64, onNotification func(subscriptionNotification)) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	t.applyHeaders(request, ModernProtocol, "subscriptions/listen", "", "", nil)
	response, err := t.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer response.Body.Close()

	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if response.StatusCode < 200 || response.StatusCode >= 300 || !strings.Contains(contentType, "text/event-stream") {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if readErr != nil {
			return readErr
		}
		if rpc, parseErr := parseHTTPRPC(body, response.Header.Get("Content-Type")); parseErr == nil && rpc.Error != nil {
			if rpc.Error.Code == -32601 {
				return ErrSubscriptionsUnsupported
			}
			return &ProtocolError{Code: rpc.Error.Code, Message: rpc.Error.Message, Data: rpc.Error.Data}
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("upstream subscription HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("upstream subscriptions/listen requires text/event-stream, got %q", response.Header.Get("Content-Type"))
	}

	return consumeSubscriptionSSE(ctx, response.Body, requestID, onNotification)
}

func consumeSubscriptionSSE(ctx context.Context, reader io.Reader, requestID int64, onNotification func(subscriptionNotification)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	dataLines := []string{}
	acknowledged := false
	expectedID := strconv.FormatInt(requestID, 10)

	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		var message subscriptionNotification
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			return fmt.Errorf("decode upstream subscription SSE notification: %w", err)
		}
		if len(message.ID) != 0 {
			return errors.New("upstream subscription stream returned a JSON-RPC response instead of a notification")
		}
		if !acknowledged {
			if message.Method != "notifications/subscriptions/acknowledged" {
				return fmt.Errorf("upstream subscription first notification is %q, expected notifications/subscriptions/acknowledged", message.Method)
			}
			if subscriptionID(message.Params) != expectedID {
				return errors.New("upstream subscription acknowledgment id mismatch")
			}
			notifications, _ := message.Params["notifications"].(map[string]any)
			accepted, _ := notifications["toolsListChanged"].(bool)
			if !accepted {
				return ErrSubscriptionsUnsupported
			}
			acknowledged = true
			return nil
		}
		if subscriptionID(message.Params) != expectedID {
			return errors.New("upstream subscription notification id mismatch")
		}
		onNotification(message)
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
		dataLines = append(dataLines, value)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if !acknowledged {
		return errors.New("upstream subscription stream closed before acknowledgment")
	}
	return io.EOF
}

func subscriptionID(params map[string]any) string {
	meta, _ := params["_meta"].(map[string]any)
	value := meta["io.modelcontextprotocol/subscriptionId"]
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return typed.String()
	}
	return ""
}

func toolsListChangedCapability(capabilities map[string]any) bool {
	tools, _ := capabilities["tools"].(map[string]any)
	value, _ := tools["listChanged"].(bool)
	return value
}
