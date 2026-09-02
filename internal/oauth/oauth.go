package oauth

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
)

var (
	ErrCredentialNotFound = errors.New("oauth credential not found")
	ErrLoginRequired      = errors.New("oauth login required")
)

type Credential struct {
	ServerID           string    `json:"server_id"`
	ServerURL          string    `json:"server_url"`
	Resource           string    `json:"resource"`
	Issuer             string    `json:"issuer"`
	Registration       string    `json:"registration"`
	ClientID           string    `json:"client_id"`
	ClientSecret       string    `json:"client_secret,omitempty"`
	ClientSecretEnvVar string    `json:"client_secret_env_var,omitempty"`
	ClientMetadataURL  string    `json:"client_metadata_url,omitempty"`
	AuthorizationURL   string    `json:"authorization_url"`
	TokenEndpoint      string    `json:"token_endpoint"`
	TokenAuthMethod    string    `json:"token_auth_method,omitempty"`
	Scopes             []string  `json:"scopes,omitempty"`
	AccessToken        string    `json:"access_token"`
	RefreshToken       string    `json:"refresh_token,omitempty"`
	TokenType          string    `json:"token_type,omitempty"`
	ExpiresAt          time.Time `json:"expires_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Status struct {
	ServerID        string     `json:"server_id"`
	Configured      bool       `json:"configured"`
	Issuer          string     `json:"issuer,omitempty"`
	Resource        string     `json:"resource,omitempty"`
	Registration    string     `json:"registration,omitempty"`
	ClientID        string     `json:"client_id,omitempty"`
	Scopes          []string   `json:"scopes,omitempty"`
	HasRefreshToken bool       `json:"has_refresh_token"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Expired         bool       `json:"expired"`
}

type RuntimeConfig struct {
	ServerID  string
	ServerURL string
}

type LoginConfig struct {
	ServerID           string
	ServerURL          string
	Scope              string
	Issuer             string
	ClientID           string
	ClientSecretEnvVar string
	ClientMetadataURL  string
}

type LoginOptions struct {
	ExtraScope string
	OnURL      func(string) error
}

type Store struct {
	mu      sync.Mutex
	path    string
	client  *http.Client
	secrets *secretstore.Store
}

func Path() string {
	return configformat.StructuredPath(configformat.RootPath(), "oauth")
}

func NewStore(path string) *Store {
	return NewStoreWithClient(path, &http.Client{Timeout: 30 * time.Second})
}

func (s *Store) Migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.readLocked()
	return err
}

func NewStoreWithClient(path string, client *http.Client) *Store {
	if strings.TrimSpace(path) == "" {
		path = Path()
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Store{path: path, client: client, secrets: secretstore.New(filepath.Dir(path))}
}
