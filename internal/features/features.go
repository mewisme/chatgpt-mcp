package features

import "encoding/json"

type Feature struct {
	Active bool `json:"active"`
}

type Config struct {
	Ponytail Feature `json:"ponytail"`
	Caveman  Feature `json:"caveman"`
}

func Default() Config {
	return Config{Ponytail: Feature{Active: true}, Caveman: Feature{Active: true}}
}

func (f *Feature) UnmarshalJSON(data []byte) error {
	var value struct {
		Active  *bool `json:"active"`
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Active != nil {
		f.Active = *value.Active
	} else if value.Enabled != nil {
		f.Active = *value.Enabled
	}
	return nil
}
