package upstream

import (
	"errors"
	"regexp"
	"strings"
)

type AuthConfig struct {
	Type  string `json:"type"`
	Scope string `json:"scope,omitempty"`
}

type Server struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Transport         string            `json:"transport"`
	Enabled           bool              `json:"enabled"`
	Command           string            `json:"command,omitempty"`
	Args              []string          `json:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	CWD               string            `json:"cwd,omitempty"`
	URL               string            `json:"url,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	BearerTokenEnvVar string            `json:"bearer_token_env_var,omitempty"`
	Auth              AuthConfig        `json:"auth,omitempty"`
	ToolPrefix        string            `json:"tool_prefix,omitempty"`
	Expose            string            `json:"expose,omitempty"`
	Tools             []string          `json:"tools,omitempty"`
	DisabledTools     []string          `json:"disabled_tools,omitempty"`
	IdleTimeoutSec    int               `json:"idle_timeout_sec,omitempty"`
}

var invalidPrefix = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func NormalizeServer(value Server) (Server, error) {
	value.ID = strings.TrimSpace(value.ID)
	if value.ID == "" {
		return Server{}, errors.New("upstream server id is required")
	}
	value.Name = strings.TrimSpace(value.Name)
	if value.Name == "" {
		value.Name = value.ID
	}
	value.Transport = strings.ToLower(strings.TrimSpace(value.Transport))
	if value.Transport != "stdio" && value.Transport != "http" {
		return Server{}, errors.New("upstream transport must be stdio or http")
	}
	value.Command = strings.TrimSpace(value.Command)
	value.URL = strings.TrimSpace(value.URL)
	value.CWD = strings.TrimSpace(value.CWD)
	value.BearerTokenEnvVar = strings.TrimSpace(value.BearerTokenEnvVar)
	if value.Transport == "stdio" && value.Command == "" {
		return Server{}, errors.New("stdio upstream requires command")
	}
	if value.Transport == "http" && value.URL == "" {
		return Server{}, errors.New("http upstream requires url")
	}
	value.ToolPrefix = invalidPrefix.ReplaceAllString(strings.TrimSpace(value.ToolPrefix), "_")
	if value.ToolPrefix == "" {
		value.ToolPrefix = invalidPrefix.ReplaceAllString(value.ID, "_")
	}
	switch value.Expose {
	case "", "all":
		value.Expose = "all"
	case "none", "meta_only", "allowlist":
	default:
		return Server{}, errors.New("upstream expose must be none, meta_only, allowlist, or all")
	}
	if !value.Enabled && value.Expose == "all" {
		value.Expose = "none"
	}
	if value.IdleTimeoutSec == 0 {
		value.IdleTimeoutSec = 600
	}
	if value.Auth.Type == "" {
		if value.Transport == "http" {
			value.Auth.Type = "auto"
		} else {
			value.Auth.Type = "none"
		}
	}
	switch value.Auth.Type {
	case "auto", "oauth", "none":
	default:
		return Server{}, errors.New("upstream auth type must be auto, oauth, or none")
	}
	if value.Args == nil {
		value.Args = []string{}
	}
	if value.Env == nil {
		value.Env = map[string]string{}
	}
	if value.Headers == nil {
		value.Headers = map[string]string{}
	}
	if value.Tools == nil {
		value.Tools = []string{}
	}
	if value.DisabledTools == nil {
		value.DisabledTools = []string{}
	}
	return value, nil
}
