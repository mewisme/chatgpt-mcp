package oauth

type Client struct {
	ID          string
	RedirectURL string
}

func NewClient(id, redirect string) Client { return Client{ID: id, RedirectURL: redirect} }
