package upstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type mcpServersDocument struct {
	Servers map[string]mcpServerJSON `json:"mcpServers"`
}

type mcpServerJSON struct {
	Name                string            `json:"name,omitempty"`
	Transport           string            `json:"transport,omitempty"`
	Enabled             bool              `json:"enabled"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	CWD                 string            `json:"cwd,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	BearerTokenEnvVar   string            `json:"bearer_token_env_var,omitempty"`
	Auth                AuthConfig        `json:"auth,omitempty"`
	ToolPrefix          string            `json:"tool_prefix,omitempty"`
	Expose              string            `json:"expose,omitempty"`
	Tools               []string          `json:"tools,omitempty"`
	DisabledTools       []string          `json:"disabled_tools,omitempty"`
	AllowPrivateNetwork bool              `json:"allow_private_network,omitempty"`
	IdleTimeoutSec      int               `json:"idle_timeout_sec,omitempty"`
}

func ParseMCPServersJSON(data []byte) ([]Server, error) {
	root, err := decodeMCPJSON(data)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		key      string
		fallback string
		value    any
	}
	switch value := root.(type) {
	case []any:
		for index, item := range value {
			entries = append(entries, struct {
				key      string
				fallback string
				value    any
			}{key: strconv.Itoa(index + 1), value: item})
		}
	case map[string]any:
		if raw, ok := value["mcpServers"]; ok {
			servers, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("mcpServers must be an object keyed by server ID")
			}
			keys := make([]string, 0, len(servers))
			for key := range servers {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				entries = append(entries, struct {
					key      string
					fallback string
					value    any
				}{key: key, fallback: key, value: servers[key]})
			}
		} else {
			entries = append(entries, struct {
				key      string
				fallback string
				value    any
			}{key: "server", value: value})
		}
	default:
		return nil, errors.New("top-level MCP server JSON must be an object or array")
	}
	if len(entries) == 0 {
		return nil, errors.New("MCP server JSON contains no servers")
	}
	result := make([]Server, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		server, err := parseMCPServerJSONEntry(entry.value, entry.fallback)
		if err != nil {
			return nil, fmt.Errorf("server %s: %w", entry.key, err)
		}
		if seen[server.ID] {
			return nil, fmt.Errorf("duplicate MCP server ID: %s", server.ID)
		}
		seen[server.ID] = true
		result = append(result, server)
	}
	return result, nil
}

func MarshalMCPServersJSON(servers []Server) ([]byte, error) {
	values := make(map[string]mcpServerJSON, len(servers))
	for _, server := range servers {
		normalized, err := NormalizeServer(server)
		if err != nil {
			return nil, err
		}
		if _, exists := values[normalized.ID]; exists {
			return nil, fmt.Errorf("duplicate MCP server ID: %s", normalized.ID)
		}
		values[normalized.ID] = mcpServerJSON{
			Name: normalized.Name, Transport: normalized.Transport, Enabled: normalized.Enabled,
			Command: normalized.Command, Args: normalized.Args, Env: normalized.Env, CWD: normalized.CWD,
			URL: normalized.URL, Headers: normalized.Headers, BearerTokenEnvVar: normalized.BearerTokenEnvVar,
			Auth: normalized.Auth, ToolPrefix: normalized.ToolPrefix, Expose: normalized.Expose,
			Tools: normalized.Tools, DisabledTools: normalized.DisabledTools,
			AllowPrivateNetwork: normalized.AllowPrivateNetwork, IdleTimeoutSec: normalized.IdleTimeoutSec,
		}
	}
	return json.MarshalIndent(mcpServersDocument{Servers: values}, "", "  ")
}

func decodeMCPJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		if syntax, ok := err.(*json.SyntaxError); ok {
			return nil, fmt.Errorf("invalid MCP server JSON at byte %d", syntax.Offset)
		}
		return nil, errors.New("invalid MCP server JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("MCP server JSON must contain exactly one top-level value")
	}
	return value, nil
}

func parseMCPServerJSONEntry(value any, fallbackID string) (Server, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return Server{}, errors.New("server entry must be an object")
	}
	id, err := mcpJSONString(object, "id")
	if err != nil {
		return Server{}, err
	}
	if id == "" {
		id = strings.TrimSpace(fallbackID)
	}
	command, err := mcpJSONString(object, "command")
	if err != nil {
		return Server{}, err
	}
	url, err := mcpJSONString(object, "url")
	if err != nil {
		return Server{}, err
	}
	transport, err := mcpJSONTransport(object, command, url)
	if err != nil {
		return Server{}, err
	}
	enabled, err := mcpJSONEnabled(object)
	if err != nil {
		return Server{}, err
	}
	server := Server{ID: id, Command: command, URL: url, Transport: transport, Enabled: enabled}
	if server.Name, err = mcpJSONString(object, "name"); err != nil {
		return Server{}, err
	}
	if server.CWD, err = mcpJSONString(object, "cwd"); err != nil {
		return Server{}, err
	}
	if server.BearerTokenEnvVar, err = mcpJSONString(object, "bearer_token_env_var"); err != nil {
		return Server{}, err
	}
	if server.ToolPrefix, err = mcpJSONString(object, "tool_prefix"); err != nil {
		return Server{}, err
	}
	if server.Expose, err = mcpJSONString(object, "expose"); err != nil {
		return Server{}, err
	}
	if server.Args, err = mcpJSONStringSlice(object, "args"); err != nil {
		return Server{}, err
	}
	if server.Tools, err = mcpJSONStringSlice(object, "tools"); err != nil {
		return Server{}, err
	}
	if _, exists := object["disabled_tools"]; exists {
		server.DisabledTools, err = mcpJSONStringSlice(object, "disabled_tools")
	} else {
		server.DisabledTools, err = mcpJSONStringSlice(object, "disabledTools")
	}
	if err != nil {
		return Server{}, err
	}
	if server.Env, err = mcpJSONStringMap(object, "env"); err != nil {
		return Server{}, err
	}
	if server.Headers, err = mcpJSONStringMap(object, "headers"); err != nil {
		return Server{}, err
	}
	if server.IdleTimeoutSec, err = mcpJSONNonNegativeInt(object, "idle_timeout_sec"); err != nil {
		return Server{}, err
	}
	if allowPrivate, set, err := mcpJSONBool(object, "allow_private_network"); err != nil {
		return Server{}, err
	} else if set {
		server.AllowPrivateNetwork = allowPrivate
	}
	if server.Auth, err = mcpJSONAuth(object); err != nil {
		return Server{}, err
	}
	normalized, err := NormalizeServer(server)
	if err != nil {
		return Server{}, err
	}
	return normalized, nil
}

func mcpJSONTransport(object map[string]any, command, url string) (string, error) {
	transport, err := mcpJSONString(object, "transport")
	if err != nil {
		return "", err
	}
	typeValue, err := mcpJSONString(object, "type")
	if err != nil {
		return "", err
	}
	transport, err = normalizeMCPTransportAlias(transport)
	if err != nil {
		return "", err
	}
	typeValue, err = normalizeMCPTransportAlias(typeValue)
	if err != nil {
		return "", err
	}
	if transport != "" && typeValue != "" && transport != typeValue {
		return "", errors.New("transport and type declare different transports")
	}
	if transport == "" {
		transport = typeValue
	}
	if transport != "" {
		return transport, nil
	}
	switch {
	case command != "" && url != "":
		return "", errors.New("transport is ambiguous when both command and url are set")
	case command != "":
		return "stdio", nil
	case url != "":
		return "http", nil
	default:
		return "", errors.New("transport could not be inferred; provide command or url")
	}
}

func normalizeMCPTransportAlias(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "stdio":
		return "stdio", nil
	case "http", "streamable-http", "streamable_http":
		return "http", nil
	default:
		return "", fmt.Errorf("unsupported transport: %s", strings.TrimSpace(value))
	}
}

func mcpJSONEnabled(object map[string]any) (bool, error) {
	enabled, enabledSet, err := mcpJSONBool(object, "enabled")
	if err != nil {
		return false, err
	}
	disabled, disabledSet, err := mcpJSONBool(object, "disabled")
	if err != nil {
		return false, err
	}
	if enabledSet && disabledSet && enabled == disabled {
		return false, errors.New("enabled and disabled declarations conflict")
	}
	if enabledSet {
		return enabled, nil
	}
	if disabledSet {
		return !disabled, nil
	}
	return true, nil
}

func mcpJSONAuth(object map[string]any) (AuthConfig, error) {
	value, exists := object["auth"]
	if !exists || value == nil {
		return AuthConfig{}, nil
	}
	if text, ok := value.(string); ok {
		return AuthConfig{Type: strings.TrimSpace(text)}, nil
	}
	auth, ok := value.(map[string]any)
	if !ok {
		return AuthConfig{}, errors.New("auth must be a string or object")
	}
	typeValue, err := mcpJSONString(auth, "type")
	if err != nil {
		return AuthConfig{}, fmt.Errorf("auth.%w", err)
	}
	scope, err := mcpJSONString(auth, "scope")
	if err != nil {
		return AuthConfig{}, fmt.Errorf("auth.%w", err)
	}
	return AuthConfig{Type: typeValue, Scope: scope}, nil
}

func mcpJSONString(object map[string]any, key string) (string, error) {
	value, exists := object[key]
	if !exists || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(text), nil
}

func mcpJSONBool(object map[string]any, key string) (bool, bool, error) {
	value, exists := object[key]
	if !exists || value == nil {
		return false, false, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, true, fmt.Errorf("%s must be a boolean", key)
	}
	return result, true, nil
}

func mcpJSONStringSlice(object map[string]any, key string) ([]string, error) {
	value, exists := object[key]
	if !exists || value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", key)
		}
		if text = strings.TrimSpace(text); text != "" {
			result = append(result, text)
		}
	}
	return result, nil
}

func mcpJSONStringMap(object map[string]any, key string) (map[string]string, error) {
	value, exists := object[key]
	if !exists || value == nil {
		return nil, nil
	}
	items, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object of string values", key)
	}
	result := make(map[string]string, len(items))
	for name, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a string", key, name)
		}
		result[name] = text
	}
	return result, nil
}

func mcpJSONNonNegativeInt(object map[string]any, key string) (int, error) {
	value, exists := object[key]
	if !exists || value == nil {
		return 0, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 || int64(int(parsed)) != parsed {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return int(parsed), nil
}
