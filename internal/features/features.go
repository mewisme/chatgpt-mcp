package features

import "encoding/json"

type Feature struct {
	Active bool `json:"active"`
}

type PonytailFeature struct {
	Active bool   `json:"active"`
	Mode   string `json:"mode"`
}

type Config struct {
	Ponytail PonytailFeature `json:"ponytail"`
	Caveman  Feature         `json:"caveman"`
}

func Default() Config {
	return Config{Ponytail: PonytailFeature{Active: true, Mode: "full"}, Caveman: Feature{Active: true}}
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

func (f *PonytailFeature) UnmarshalJSON(data []byte) error {
	var value struct {
		Active  *bool   `json:"active"`
		Enabled *bool   `json:"enabled"`
		Mode    *string `json:"mode"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Active != nil {
		f.Active = *value.Active
	} else if value.Enabled != nil {
		f.Active = *value.Enabled
	}
	if value.Mode != nil {
		f.Mode = *value.Mode
	}
	return nil
}
