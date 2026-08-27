package mcp

func ValidateRequest(req Request) error {
	if req.JSONRPC != "2.0" {
		return NewError(ErrInvalidRequest, "invalid jsonrpc version")
	}
	if req.Method == "" {
		return NewError(ErrInvalidRequest, "missing method")
	}
	return nil
}
