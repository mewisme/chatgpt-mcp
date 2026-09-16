package runtimeplugin

type DescribeResult struct {
	Provider string         `json:"provider"`
	Name     string         `json:"name"`
	Targets  []Target       `json:"targets"`
	Settings map[string]any `json:"settings,omitempty"`
}

type Target struct {
	ID                          string `json:"id"`
	OriginKind                  string `json:"origin_kind"`
	RequiresAuthenticatedOrigin bool   `json:"requires_authenticated_origin"`
}

type StatusResult struct {
	State   string         `json:"state,omitempty"`
	Targets []TargetStatus `json:"targets,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type TargetStatus struct {
	Target     string `json:"target"`
	Desired    bool   `json:"desired,omitempty"`
	Running    bool   `json:"running,omitempty"`
	Ready      bool   `json:"ready,omitempty"`
	Restarting bool   `json:"restarting,omitempty"`
	URL        string `json:"url,omitempty"`
	Origin     string `json:"origin,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	Ephemeral  bool   `json:"ephemeral,omitempty"`
}

type StartParams struct {
	Target     string         `json:"target"`
	Origin     string         `json:"origin"`
	PublicPath string         `json:"public_path,omitempty"`
	Settings   map[string]any `json:"settings,omitempty"`
}

type StopParams struct {
	Target string `json:"target"`
}

type LogEvent struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Target   string `json:"target,omitempty"`
}

type Event struct {
	Name string
	Data []byte
}
