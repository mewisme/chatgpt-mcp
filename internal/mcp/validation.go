package mcp

import "encoding/json"

func ValidateRequest(req Request) error {
	if req.JSONRPC != "2.0" {
		return NewError(ErrInvalidRequest, "invalid jsonrpc version")
	}
	if req.Method == "" {
		return NewError(ErrInvalidRequest, "missing method")
	}
	return nil
}

func DecodeParams(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, NewError(ErrInvalidParams, "params must be an object")
	}
	return params, nil
}

func ValidateParams(method string, params map[string]any) error {
	switch method {
	case "initialize":
		version, ok := params["protocolVersion"].(string)
		if !ok || version == "" {
			return NewError(ErrInvalidParams, "missing protocolVersion")
		}
		if version != SupportedProtocolVersion {
			return NewError(ErrInvalidParams, "unsupported protocolVersion")
		}
		clientInfo, ok := params["clientInfo"].(map[string]any)
		if !ok {
			return NewError(ErrInvalidParams, "missing clientInfo")
		}
		name, _ := clientInfo["name"].(string)
		version, _ = clientInfo["version"].(string)
		if name == "" || version == "" {
			return NewError(ErrInvalidParams, "invalid clientInfo")
		}
		if capabilities, exists := params["capabilities"]; exists {
			if _, ok := capabilities.(map[string]any); !ok {
				return NewError(ErrInvalidParams, "capabilities must be an object")
			}
		}
	case "tools/call":
		name, ok := params["name"].(string)
		if !ok || name == "" {
			return NewError(ErrInvalidParams, "missing tool name")
		}
		if args, exists := params["arguments"]; exists {
			if _, ok := args.(map[string]any); !ok {
				return NewError(ErrInvalidParams, "arguments must be an object")
			}
		}
	}
	return nil
}
