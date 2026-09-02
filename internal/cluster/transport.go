package cluster

import "context"

type Transport interface {
	Connect(context.Context, Advertisement) (Session, error)
}

type Session interface {
	Advertise(context.Context, Advertisement) error
	Snapshot(context.Context) (Snapshot, error)
	Send(context.Context, Frame) error
	Receive(context.Context) (Frame, error)
	Close() error
}
