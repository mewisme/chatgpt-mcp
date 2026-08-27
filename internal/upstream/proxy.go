package upstream

import "encoding/json"

func ProxyName(server, tool string) string { return server + "__" + tool }

func ForwardResult(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
