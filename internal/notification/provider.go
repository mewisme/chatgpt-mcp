package notification

import (
	"context"
	"errors"
)

type Capabilities struct {
	Notification bool
	Actions      bool
	OpenTerminal bool
}

type Notification struct {
	RequestID  string
	Title      string
	Body       string
	OpenAction bool
}

type Provider interface {
	Available(context.Context) bool
	Capabilities(context.Context) Capabilities
	Send(context.Context, Notification) error
}

type unavailableProvider struct{}

func UnavailableProvider() Provider { return unavailableProvider{} }

func (unavailableProvider) Available(context.Context) bool { return false }
func (unavailableProvider) Capabilities(context.Context) Capabilities {
	return Capabilities{}
}
func (unavailableProvider) Send(context.Context, Notification) error {
	return errors.New("desktop notifications unavailable")
}
