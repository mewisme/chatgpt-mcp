package features

type Feature struct {
	Enabled bool `json:"enabled"`
}

type Config struct {
	Ponytail Feature `json:"ponytail"`
	Caveman  Feature `json:"caveman"`
}

func Default() Config {
	return Config{Ponytail: Feature{Enabled: true}, Caveman: Feature{Enabled: true}}
}
