package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ModernProtocol = "2026-07-28"
	LegacyProtocol = "2025-11-25"
)

type Tool struct {
	Server       string         `json:"server,omitempty"`
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
	Execution    map[string]any `json:"execution,omitempty"`
	Meta         map[string]any `json:"_meta,omitempty"`
}

type Content struct {
	Type string         `json:"type"`
	Text string         `json:"text,omitempty"`
	Data any            `json:"data,omitempty"`
	Raw  map[string]any `json:"-"`
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Raw = raw
	if value, ok := raw["type"].(string); ok {
		c.Type = value
	}
	if value, ok := raw["text"].(string); ok {
		c.Text = value
	}
	if value, ok := raw["data"]; ok {
		c.Data = value
	}
	return nil
}

func (c Content) MarshalJSON() ([]byte, error) {
	if c.Raw != nil {
		return json.Marshal(c.Raw)
	}
	return json.Marshal(map[string]any{"type": c.Type, "text": c.Text, "data": c.Data})
}

type CallResult struct {
	Content           []Content      `json:"content,omitempty"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
	ResultType        string         `json:"resultType,omitempty"`
	RequestState      string         `json:"requestState,omitempty"`
	InputRequests     map[string]any `json:"inputRequests,omitempty"`
}

type Client interface {
	Connect(context.Context, Server) error
	Close(context.Context, string) error
	Tools(context.Context, string) ([]Tool, error)
	Call(context.Context, string, string, map[string]any) (CallResult, error)
	PID(string) int
}

type NativeClient struct {
	mu          sync.Mutex
	connections map[string]*rpcConnection
	httpClient  *http.Client
}

type rpcConnection struct {
	mu      sync.Mutex
	server  Server
	era     string
	session string
	pid     int
	stdio   *stdioTransport
	http    *httpTransport
	tools   map[string]Tool
	nextID  atomic.Int64
}

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
	mu     sync.Mutex
}

type httpTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
}

func NewNativeClient() *NativeClient {
	return &NativeClient{
		connections: map[string]*rpcConnection{},
		httpClient:  &http.Client{Timeout: 0},
	}
}

func (c *NativeClient) Connect(ctx context.Context, server Server) error {
	server, err := NormalizeServer(server)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if existing := c.connections[server.ID]; existing != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	connection, err := c.createConnection(ctx, server)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if existing := c.connections[server.ID]; existing != nil {
		c.mu.Unlock()
		_ = connection.close(context.Background())
		return nil
	}
	c.connections[server.ID] = connection
	c.mu.Unlock()
	return nil
}

func (c *NativeClient) Close(ctx context.Context, id string) error {
	c.mu.Lock()
	connection := c.connections[id]
	delete(c.connections, id)
	c.mu.Unlock()
	if connection == nil {
		return nil
	}
	return connection.close(ctx)
}

func (c *NativeClient) Tools(ctx context.Context, id string) ([]Tool, error) {
	connection, err := c.connection(id)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := connection.call(ctx, "tools/list", "", map[string]any{}, &result); err != nil {
		return nil, err
	}
	modernHTTP := connection.isModernHTTP()
	filtered := make([]Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tool.Server = id
		if modernHTTP {
			if _, err := toolHeaderSpecs(tool.InputSchema); err != nil {
				continue
			}
		}
		filtered = append(filtered, tool)
	}
	connection.rememberTools(filtered)
	return filtered, nil
}

func (c *NativeClient) Call(ctx context.Context, id, tool string, args map[string]any) (CallResult, error) {
	return c.CallWithInput(ctx, id, tool, args, "", nil)
}

func (c *NativeClient) CallWithInput(ctx context.Context, id, tool string, args map[string]any, requestState string, inputResponses map[string]any) (CallResult, error) {
	connection, err := c.connection(id)
	if err != nil {
		return CallResult{}, err
	}
	headers, err := c.headersForCall(ctx, connection, id, tool, args)
	if err != nil {
		return CallResult{}, err
	}
	params := map[string]any{"name": tool, "arguments": args}
	if requestState != "" {
		params["requestState"] = requestState
	}
	if inputResponses != nil {
		params["inputResponses"] = inputResponses
	}
	var result CallResult
	err = connection.callWithHeaders(ctx, "tools/call", tool, params, headers, &result)
	if isHeaderMismatch(err) && connection.isModernHTTP() {
		if _, refreshErr := c.Tools(ctx, id); refreshErr != nil {
			return CallResult{}, fmt.Errorf("refresh tools after HeaderMismatch: %w", refreshErr)
		}
		headers, err = c.headersForCall(ctx, connection, id, tool, args)
		if err != nil {
			return CallResult{}, err
		}
		err = connection.callWithHeaders(ctx, "tools/call", tool, params, headers, &result)
	}
	if err != nil {
		return CallResult{}, err
	}
	if result.ResultType != "input_required" && result.Content == nil {
		result.Content = []Content{}
	}
	return result, nil
}

func (c *NativeClient) headersForCall(ctx context.Context, connection *rpcConnection, id, tool string, args map[string]any) (map[string]string, error) {
	if !connection.isModernHTTP() {
		return nil, nil
	}
	definition, ok := connection.toolDefinition(tool)
	if !ok {
		if _, err := c.Tools(ctx, id); err != nil {
			return nil, err
		}
		definition, ok = connection.toolDefinition(tool)
		if !ok {
			return nil, fmt.Errorf("upstream tool definition not found: %s/%s", id, tool)
		}
	}
	return mirroredToolHeaders(definition, args)
}

func (c *NativeClient) PID(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if connection := c.connections[id]; connection != nil {
		return connection.pid
	}
	return 0
}

func (c *NativeClient) connection(id string) (*rpcConnection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.connections[id]
	if value == nil {
		return nil, fmt.Errorf("upstream not connected: %s", id)
	}
	return value, nil
}

func (c *rpcConnection) isModernHTTP() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.http != nil && c.era == ModernProtocol
}

func (c *rpcConnection) rememberTools(tools []Tool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools = make(map[string]Tool, len(tools))
	for _, tool := range tools {
		c.tools[tool.Name] = tool
	}
}

func (c *rpcConnection) toolDefinition(name string) (Tool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tool, ok := c.tools[name]
	return tool, ok
}

func (c *NativeClient) createConnection(ctx context.Context, server Server) (*rpcConnection, error) {
	connection := &rpcConnection{server: server, tools: map[string]Tool{}}
	if server.Transport == "stdio" {
		transport, err := startStdio(server)
		if err != nil {
			return nil, err
		}
		connection.stdio = transport
		if transport.cmd.Process != nil {
			connection.pid = transport.cmd.Process.Pid
		}
	} else {
		headers := map[string]string{}
		for key, value := range server.Headers {
			headers[key] = value
		}
		if env := strings.TrimSpace(server.BearerTokenEnvVar); env != "" {
			token := strings.TrimSpace(os.Getenv(env))
			if token == "" {
				return nil, fmt.Errorf("missing bearer token environment variable for %s: %s", server.ID, env)
			}
			if !hasHeader(headers, "Authorization") {
				headers["Authorization"] = "Bearer " + token
			}
		}
		connection.http = &httpTransport{url: server.URL, headers: headers, client: c.httpClient}
	}
	if err := connection.negotiate(ctx); err != nil {
		_ = connection.close(context.Background())
		return nil, err
	}
	return connection, nil
}

func (c *rpcConnection) negotiate(ctx context.Context) error {
	var discovery map[string]any
	err := c.callEra(ctx, ModernProtocol, "server/discover", "", map[string]any{}, &discovery)
	if err == nil {
		c.era = ModernProtocol
		return nil
	}
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != -32601 {
		return fmt.Errorf("modern upstream discovery failed: %w", err)
	}

	params := map[string]any{
		"protocolVersion": LegacyProtocol,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "chatgpt-mcp", "version": "1.0.0"},
	}
	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.callEra(ctx, LegacyProtocol, "initialize", "", params, &initialized); err != nil {
		return fmt.Errorf("legacy upstream initialize failed: %w", err)
	}
	c.era = initialized.ProtocolVersion
	if c.era == "" {
		c.era = LegacyProtocol
	}
	return c.notify(ctx, "notifications/initialized", map[string]any{})
}

func (c *rpcConnection) call(ctx context.Context, method, name string, params map[string]any, target any) error {
	return c.callWithHeaders(ctx, method, name, params, nil, target)
}

func (c *rpcConnection) callWithHeaders(ctx context.Context, method, name string, params map[string]any, headers map[string]string, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	era := c.era
	if era == "" {
		era = ModernProtocol
	}
	return c.callEraLocked(ctx, era, method, name, params, headers, target)
}

func (c *rpcConnection) callEra(ctx context.Context, era, method, name string, params map[string]any, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callEraLocked(ctx, era, method, name, params, nil, target)
}

func (c *rpcConnection) callEraLocked(ctx context.Context, era, method, name string, params map[string]any, headers map[string]string, target any) error {
	id := c.nextID.Add(1)
	value := cloneMap(params)
	if era == ModernProtocol {
		value["_meta"] = requestMeta(ctx)
	}
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: value}
	response, err := c.roundTrip(ctx, request, era, name, headers)
	if err != nil {
		return err
	}
	if response.Error != nil {
		return &ProtocolError{Code: response.Error.Code, Message: response.Error.Message, Data: response.Error.Data}
	}
	if target == nil || len(response.Result) == 0 {
		return nil
	}
	return json.Unmarshal(response.Result, target)
}

func (c *rpcConnection) notify(ctx context.Context, method string, params map[string]any) error {
	request := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if c.stdio != nil {
		c.stdio.mu.Lock()
		defer c.stdio.mu.Unlock()
		_, err = c.stdio.stdin.Write(append(data, '\n'))
		return err
	}
	return c.http.notify(ctx, data, c.era, method, c.session)
}

func (c *rpcConnection) roundTrip(ctx context.Context, request rpcRequest, era, name string, headers map[string]string) (rpcResponse, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return rpcResponse{}, err
	}
	if c.stdio != nil {
		return c.stdio.roundTrip(ctx, data, request.ID)
	}
	response, session, err := c.http.roundTrip(ctx, data, era, request.Method, name, c.session, headers)
	if session != "" {
		c.session = session
	}
	return response, err
}

func (c *rpcConnection) close(ctx context.Context) error {
	if c.stdio != nil {
		return c.stdio.close(ctx)
	}
	return nil
}

func startStdio(server Server) (*stdioTransport, error) {
	cmd := exec.Command(server.Command, server.Args...)
	if server.CWD != "" {
		cmd.Dir = server.CWD
	}
	env := envMap(os.Environ())
	for key, value := range server.Env {
		env[key] = value
	}
	cmd.Env = envSlice(env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	value := &stdioTransport{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}
	cmd.Stderr = &value.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return value, nil
}

func (t *stdioTransport) roundTrip(ctx context.Context, data []byte, id int64) (rpcResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.stdin.Write(append(data, '\n')); err != nil {
		return rpcResponse{}, t.withStderr(err)
	}
	type readValue struct {
		response rpcResponse
		err      error
	}
	done := make(chan readValue, 1)
	go func() {
		for {
			line, err := t.stdout.ReadBytes('\n')
			if err != nil {
				done <- readValue{err: t.withStderr(err)}
				return
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var response rpcResponse
			if json.Unmarshal(line, &response) != nil {
				continue
			}
			if rawID(response.ID) != strconv.FormatInt(id, 10) {
				continue
			}
			done <- readValue{response: response}
			return
		}
	}()
	select {
	case <-ctx.Done():
		_ = t.close(context.Background())
		return rpcResponse{}, ctx.Err()
	case result := <-done:
		return result.response, result.err
	}
}

func (t *stdioTransport) close(ctx context.Context) error {
	_ = t.stdin.Close()
	if t.cmd == nil || t.cmd.Process == nil || t.cmd.ProcessState != nil {
		return nil
	}
	_ = t.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- t.cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = t.cmd.Process.Kill()
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		return t.cmd.Process.Kill()
	case <-done:
		return nil
	}
}

func (t *stdioTransport) withStderr(err error) error {
	text := strings.TrimSpace(t.stderr.String())
	if text == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, text)
}

func (t *httpTransport) roundTrip(ctx context.Context, data []byte, era, method, name, session string, headers map[string]string) (rpcResponse, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return rpcResponse{}, "", err
	}
	t.applyHeaders(request, era, method, name, session, headers)
	response, err := t.client.Do(request)
	if err != nil {
		return rpcResponse{}, "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return rpcResponse{}, "", err
	}
	sessionValue := response.Header.Get("Mcp-Session-Id")
	rpc, parseErr := parseHTTPRPC(body, response.Header.Get("Content-Type"))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if parseErr == nil && rpc.Error != nil {
			return rpc, sessionValue, nil
		}
		return rpcResponse{}, sessionValue, fmt.Errorf("upstream HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if parseErr != nil {
		return rpcResponse{}, sessionValue, parseErr
	}
	return rpc, sessionValue, nil
}

func (t *httpTransport) notify(ctx context.Context, data []byte, era, method, session string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	t.applyHeaders(request, era, method, "", session, nil)
	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("upstream notification HTTP %d", response.StatusCode)
	}
	return nil
}

func (t *httpTransport) applyHeaders(request *http.Request, era, method, name, session string, messageHeaders map[string]string) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range t.headers {
		if era == ModernProtocol && strings.HasPrefix(strings.ToLower(key), "mcp-param-") {
			continue
		}
		request.Header.Set(key, value)
	}
	if era == ModernProtocol {
		request.Header.Set("MCP-Protocol-Version", ModernProtocol)
		request.Header.Set("Mcp-Method", method)
		if name != "" {
			request.Header.Set("Mcp-Name", encodeMCPHeaderValue(name))
		}
		for key, value := range messageHeaders {
			if strings.HasPrefix(strings.ToLower(key), "mcp-param-") {
				request.Header.Set(key, value)
			}
		}
		return
	}
	if era != "" && method != "initialize" {
		request.Header.Set("MCP-Protocol-Version", era)
	}
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
}

func parseHTTPRPC(body []byte, contentType string) (rpcResponse, error) {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return parseSSERPC(body)
	}
	var response rpcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return rpcResponse{}, fmt.Errorf("decode upstream JSON-RPC response: %w", err)
	}
	return response, nil
}

func parseSSERPC(body []byte) (rpcResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	var dataLines []string
	var lastErr error
	flush := func() (rpcResponse, bool) {
		if len(dataLines) == 0 {
			return rpcResponse{}, false
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		var response rpcResponse
		if err := json.Unmarshal([]byte(payload), &response); err != nil {
			lastErr = fmt.Errorf("decode upstream SSE JSON-RPC response: %w", err)
			return rpcResponse{}, false
		}
		if len(response.ID) == 0 {
			return rpcResponse{}, false
		}
		return response, true
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if response, ok := flush(); ok {
				return response, nil
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimPrefix(line, "data:")
		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		dataLines = append(dataLines, value)
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, err
	}
	if response, ok := flush(); ok {
		return response, nil
	}
	if lastErr != nil {
		return rpcResponse{}, lastErr
	}
	return rpcResponse{}, errors.New("empty upstream SSE response")
}

type toolHeaderSpec struct {
	Argument string
	Header   string
	Type     string
}

func toolHeaderSpecs(schema map[string]any) ([]toolHeaderSpec, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	total := countMCPHeaderAnnotations(schema)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		if total > 0 {
			return nil, errors.New("x-mcp-header is only supported on top-level inputSchema properties")
		}
		return nil, nil
	}
	specs := make([]toolHeaderSpec, 0)
	seen := map[string]bool{}
	topLevel := 0
	for argument, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		rawHeader, exists := property["x-mcp-header"]
		if !exists {
			continue
		}
		topLevel++
		header, ok := rawHeader.(string)
		if !ok || header == "" || !isHTTPToken(header) {
			return nil, fmt.Errorf("invalid x-mcp-header for %s", argument)
		}
		key := strings.ToLower(header)
		if seen[key] {
			return nil, fmt.Errorf("duplicate x-mcp-header %q", header)
		}
		seen[key] = true
		kind, _ := property["type"].(string)
		switch kind {
		case "string", "integer", "boolean":
		default:
			return nil, fmt.Errorf("x-mcp-header parameter %s must use string, integer, or boolean type", argument)
		}
		specs = append(specs, toolHeaderSpec{Argument: argument, Header: "Mcp-Param-" + header, Type: kind})
	}
	if total != topLevel {
		return nil, errors.New("x-mcp-header is only supported on top-level inputSchema properties")
	}
	return specs, nil
}

func countMCPHeaderAnnotations(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		count := 0
		for key, item := range typed {
			if key == "x-mcp-header" {
				count++
			}
			count += countMCPHeaderAnnotations(item)
		}
		return count
	case []any:
		count := 0
		for _, item := range typed {
			count += countMCPHeaderAnnotations(item)
		}
		return count
	default:
		return 0
	}
}

func mirroredToolHeaders(tool Tool, args map[string]any) (map[string]string, error) {
	specs, err := toolHeaderSpecs(tool.InputSchema)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]string, len(specs))
	for _, spec := range specs {
		value, exists := args[spec.Argument]
		if !exists || value == nil {
			continue
		}
		text, err := toolHeaderPrimitive(spec.Type, value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.Argument, err)
		}
		headers[spec.Header] = encodeMCPHeaderValue(text)
	}
	return headers, nil
}

func toolHeaderPrimitive(kind string, value any) (string, error) {
	switch kind {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", errors.New("x-mcp-header value must be a string")
		}
		return text, nil
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return "", errors.New("x-mcp-header value must be a boolean")
		}
		return strconv.FormatBool(boolean), nil
	case "integer":
		return integerHeaderValue(value)
	default:
		return "", fmt.Errorf("unsupported x-mcp-header type: %s", kind)
	}
}

func integerHeaderValue(value any) (string, error) {
	switch typed := value.(type) {
	case int:
		return strconv.FormatInt(int64(typed), 10), nil
	case int8:
		return strconv.FormatInt(int64(typed), 10), nil
	case int16:
		return strconv.FormatInt(int64(typed), 10), nil
	case int32:
		return strconv.FormatInt(int64(typed), 10), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case uint:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint64:
		return strconv.FormatUint(typed, 10), nil
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return "", errors.New("x-mcp-header value must be an integer")
		}
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed {
			return "", errors.New("x-mcp-header value must be an integer")
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		value, err := typed.Float64()
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return "", errors.New("x-mcp-header value must be an integer")
		}
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	default:
		return "", errors.New("x-mcp-header value must be an integer")
	}
}

func encodeMCPHeaderValue(value string) string {
	if safeMCPHeaderValue(value) && !(strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?=")) {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func safeMCPHeaderValue(value string) bool {
	if len(value) > 0 {
		if value[0] == ' ' || value[0] == '\t' || value[len(value)-1] == ' ' || value[len(value)-1] == '\t' {
			return false
		}
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '\t' {
			continue
		}
		if char < 0x20 || char > 0x7e {
			return false
		}
	}
	return true
}

func isHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			return false
		}
	}
	return true
}

func isHeaderMismatch(err error) bool {
	var protocolErr *ProtocolError
	return errors.As(err, &protocolErr) && protocolErr.Code == -32020
}

type ProtocolError struct {
	Code    int
	Message string
	Data    any
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return "upstream protocol error"
	}
	return fmt.Sprintf("upstream protocol error %d: %s", e.Code, e.Message)
}

func requestMeta(ctx context.Context) map[string]any {
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion": ModernProtocol,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"name": "chatgpt-mcp", "version": "1.0.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	incoming := RequestMetaFromContext(ctx)
	if capabilities, ok := incoming["io.modelcontextprotocol/clientCapabilities"].(map[string]any); ok {
		meta["io.modelcontextprotocol/clientCapabilities"] = cloneMap(capabilities)
	}
	for _, key := range []string{"traceparent", "tracestate", "baggage"} {
		if value, ok := incoming[key].(string); ok && value != "" {
			meta[key] = value
		}
	}
	return meta
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value)+1)
	for key, item := range value {
		result[key] = item
	}
	return result
}

func rawID(value json.RawMessage) string {
	text := strings.TrimSpace(string(value))
	return strings.Trim(text, `"`)
}

func hasHeader(values map[string]string, name string) bool {
	for key := range values {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func envMap(values []string) map[string]string {
	result := map[string]string{}
	for _, value := range values {
		if index := strings.IndexByte(value, '='); index >= 0 {
			result[value[:index]] = value[index+1:]
		}
	}
	return result
}

func envSlice(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
