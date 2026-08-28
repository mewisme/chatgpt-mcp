package mcp

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	ProtocolVersionHeader = "MCP-Protocol-Version"
	MethodHeader          = "Mcp-Method"
	NameHeader            = "Mcp-Name"
)

func ValidateHTTPHeaders(r *http.Request, req Request, params map[string]any) *Error {
	protocolVersion := strings.TrimSpace(r.Header.Get(ProtocolVersionHeader))
	if protocolVersion == "" {
		return NewError(ErrUnsupportedProtocolVersion, "missing MCP-Protocol-Version header")
	}
	if protocolVersion != SupportedProtocolVersion {
		return NewError(ErrUnsupportedProtocolVersion, fmt.Sprintf("unsupported protocol version %q", protocolVersion))
	}

	method := strings.TrimSpace(r.Header.Get(MethodHeader))
	if method == "" {
		return NewError(ErrHeaderMismatch, "missing Mcp-Method header")
	}
	if method != req.Method {
		return NewError(ErrHeaderMismatch, "Mcp-Method header does not match request method")
	}

	expectedName := mirroredRequestName(req.Method, params)
	name := strings.TrimSpace(r.Header.Get(NameHeader))
	if expectedName != "" {
		if name == "" {
			return NewError(ErrHeaderMismatch, "missing Mcp-Name header")
		}
		if name != expectedName {
			return NewError(ErrHeaderMismatch, "Mcp-Name header does not match request parameters")
		}
	} else if name != "" {
		return NewError(ErrHeaderMismatch, "Mcp-Name header is not valid for this method")
	}

	if metaVersion := requestMetaProtocolVersion(params); metaVersion != "" && metaVersion != protocolVersion {
		return NewError(ErrHeaderMismatch, "protocol version metadata does not match MCP-Protocol-Version header")
	}
	return nil
}

func mirroredRequestName(method string, params map[string]any) string {
	switch method {
	case "tools/call":
		name, _ := params["name"].(string)
		return name
	default:
		return ""
	}
}

func requestMetaProtocolVersion(params map[string]any) string {
	meta, _ := params["_meta"].(map[string]any)
	value, _ := meta["io.modelcontextprotocol/protocolVersion"].(string)
	return value
}
