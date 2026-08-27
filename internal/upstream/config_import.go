package upstream

import (
	"encoding/json"
	"os"
)

func ImportJSON(path string) ([]Server, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var servers []Server
	err = json.Unmarshal(data, &servers)
	return servers, err
}
