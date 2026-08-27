package tunnel

type Client struct {
	ID     string
	APIKey string
}

func New(id, key string) *Client { return &Client{ID: id, APIKey: key} }

func (c *Client) Start() error { return nil }
func (c *Client) Stop() error  { return nil }
