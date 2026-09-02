package cluster

import (
	"context"
	"time"
)

type Transport interface {
	Connect(context.Context, Advertisement) (Session, error)
}

type Session interface {
	Advertise(context.Context, Advertisement) error
	Snapshot(context.Context) (Snapshot, error)
	TryAcquireLeadership(context.Context, string, time.Duration) (LeaderLease, bool, error)
	RenewLeadership(context.Context, LeaderLease, time.Duration) (LeaderLease, error)
	ReleaseLeadership(context.Context, LeaderLease) error
	Leadership(context.Context, string) (LeaderLease, bool, error)
	Send(context.Context, Frame) error
	Receive(context.Context) (Frame, error)
	Close() error
}
