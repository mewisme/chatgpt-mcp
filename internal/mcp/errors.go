package mcp

const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

func (e *Error) Error() string { return e.Message }

func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}
