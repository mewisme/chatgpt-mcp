package tools

import (
	"context"
	"fmt"
)

type Context struct {
	WorkingDirectory string
}

type InputRound struct {
	RequestState   string
	InputResponses map[string]any
}

type inputRoundContextKey struct{}

func WithInputRound(ctx context.Context, requestState string, inputResponses map[string]any) context.Context {
	if requestState == "" && inputResponses == nil {
		return ctx
	}
	return context.WithValue(ctx, inputRoundContextKey{}, InputRound{RequestState: requestState, InputResponses: inputResponses})
}

func InputRoundFromContext(ctx context.Context) InputRound {
	value, _ := ctx.Value(inputRoundContextKey{}).(InputRound)
	return value
}

func RequireWorkingDirectory(args map[string]any) error {
	value, ok := args["working_directory"].(string)
	if !ok || value == "" {
		return fmt.Errorf("working_directory is required")
	}
	return nil
}
