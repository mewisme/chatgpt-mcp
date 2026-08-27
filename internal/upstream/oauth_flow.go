package upstream

import "context"

type OAuthClient interface {
	Connect(context.Context, string) error
	Disconnect(string) error
}
