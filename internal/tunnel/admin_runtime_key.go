package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tunnelclient "github.com/openai/tunnel-client"
)

const runtimeServiceAccountName = "chatgpt-mcp tunnel runtime"

var runtimeKeyScopes = []string{"api.organization.tunnel.read", "api.organization.tunnel.use"}

type AdminProject struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

type GeneratedRuntimeKey struct {
	ProjectID        string
	ServiceAccountID string
	KeyID            string
	Value            string
}

type adminProjectPage struct {
	Data    []AdminProject `json:"data"`
	HasMore bool           `json:"has_more"`
	LastID  string         `json:"last_id,omitempty"`
	Next    string         `json:"next,omitempty"`
}

type adminServiceAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type adminServiceAccountPage struct {
	Data    []adminServiceAccount `json:"data"`
	HasMore bool                  `json:"has_more"`
	LastID  string                `json:"last_id,omitempty"`
	Next    string                `json:"next,omitempty"`
}

type adminRuntimeKeyResponse struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func ListAdminProjects(ctx context.Context, cfg Config) ([]AdminProject, error) {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return nil, errors.New("OpenAI admin key is not configured")
	}
	var page adminProjectPage
	if err := adminPlatformRequest(ctx, cfg, http.MethodGet, "/v1/organization/projects?limit=100&include_archived=false", nil, &page); err != nil {
		return nil, err
	}
	projects := make([]AdminProject, 0, len(page.Data))
	for _, project := range page.Data {
		if project.Status == "" || strings.EqualFold(project.Status, "active") {
			projects = append(projects, project)
		}
	}
	return projects, nil
}

func GenerateRuntimeKey(ctx context.Context, cfg Config, projectID string) (GeneratedRuntimeKey, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projects, err := ListAdminProjects(ctx, cfg)
		if err != nil {
			return GeneratedRuntimeKey{}, fmt.Errorf("list OpenAI projects for runtime key generation: %w", err)
		}
		projectID, err = selectRuntimeKeyProject(projects)
		if err != nil {
			return GeneratedRuntimeKey{}, err
		}
	}
	account, err := findOrCreateRuntimeServiceAccount(ctx, cfg, projectID)
	if err != nil {
		return GeneratedRuntimeKey{}, err
	}
	path := fmt.Sprintf("/v1/organization/projects/%s/service_accounts/%s/api_keys", url.PathEscape(projectID), url.PathEscape(account.ID))
	body := map[string]any{"name": "chatgpt-mcp tunnel runtime", "scopes": append([]string(nil), runtimeKeyScopes...)}
	var response adminRuntimeKeyResponse
	if err := adminPlatformRequest(ctx, cfg, http.MethodPost, path, body, &response); err != nil {
		return GeneratedRuntimeKey{}, fmt.Errorf("create OpenAI runtime API key: %w", err)
	}
	if strings.TrimSpace(response.Value) == "" {
		return GeneratedRuntimeKey{}, errors.New("OpenAI runtime API key response did not include the key value")
	}
	return GeneratedRuntimeKey{ProjectID: projectID, ServiceAccountID: account.ID, KeyID: response.ID, Value: response.Value}, nil
}

func selectRuntimeKeyProject(projects []AdminProject) (string, error) {
	if len(projects) == 0 {
		return "", errors.New("no active OpenAI project is available for runtime key generation")
	}
	if len(projects) == 1 {
		return projects[0].ID, nil
	}
	for _, project := range projects {
		if strings.EqualFold(strings.TrimSpace(project.Name), "default project") {
			return project.ID, nil
		}
	}
	return "", errors.New("multiple active OpenAI projects are available; choose a project for runtime key generation")
}

func findOrCreateRuntimeServiceAccount(ctx context.Context, cfg Config, projectID string) (adminServiceAccount, error) {
	path := fmt.Sprintf("/v1/organization/projects/%s/service_accounts?limit=100", url.PathEscape(projectID))
	var page adminServiceAccountPage
	if err := adminPlatformRequest(ctx, cfg, http.MethodGet, path, nil, &page); err != nil {
		return adminServiceAccount{}, fmt.Errorf("list OpenAI project service accounts: %w", err)
	}
	for _, account := range page.Data {
		if strings.EqualFold(strings.TrimSpace(account.Name), runtimeServiceAccountName) {
			return account, nil
		}
	}
	var account adminServiceAccount
	body := map[string]any{"name": runtimeServiceAccountName, "create_service_account_only": true}
	if err := adminPlatformRequest(ctx, cfg, http.MethodPost, fmt.Sprintf("/v1/organization/projects/%s/service_accounts", url.PathEscape(projectID)), body, &account); err != nil {
		return adminServiceAccount{}, fmt.Errorf("create OpenAI runtime service account: %w", err)
	}
	if strings.TrimSpace(account.ID) == "" {
		return adminServiceAccount{}, errors.New("OpenAI service account response did not include an id")
	}
	return account, nil
}

func adminPlatformRequest(ctx context.Context, cfg Config, method, path string, body any, output any) error {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.ControlPlaneBaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(tunnelclient.DefaultControlPlaneBaseURL, "/")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid OpenAI API base URL %q", baseURL)
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.AdminKey))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("OpenAI Admin API %s %s: %s", method, path, message)
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode OpenAI Admin API response: %w", err)
	}
	return nil
}
