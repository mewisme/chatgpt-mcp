package upstream

import "encoding/json"

type Tool struct {
	Server string `json:"server"`
	Name string `json:"name"`
	Description string `json:"description,omitempty"`
}

func ProxyName(server, tool string) string { return server + "__" + tool }

func ForwardResult(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
