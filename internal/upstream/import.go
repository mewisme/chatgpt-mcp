package upstream

import "encoding/json"

func ParseConfig(data []byte) ([]Server, error) {
	var servers []Server
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, err
	}
	return servers, nil
}
