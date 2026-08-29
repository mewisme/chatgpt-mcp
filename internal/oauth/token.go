package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    json.Number `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	Scope        string      `json:"scope"`
	Error        string      `json:"error"`
	Description  string      `json:"error_description"`
}

func (s *Store) AccessToken(ctx context.Context, config RuntimeConfig) (string, error) {
	credential, err := s.Get(config.ServerID)
	if err != nil {
		return "", err
	}
	if credential.ServerURL != config.ServerURL {
		return "", fmt.Errorf("%w: stored credential is bound to a different MCP server URL", ErrLoginRequired)
	}
	if credential.AccessToken != "" && (credential.ExpiresAt.IsZero() || time.Now().Add(30*time.Second).Before(credential.ExpiresAt)) {
		return credential.AccessToken, nil
	}
	if credential.RefreshToken == "" {
		return "", fmt.Errorf("%w: access token expired and no refresh token is available", ErrLoginRequired)
	}
	secret, err := credentialSecret(credential)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {credential.RefreshToken},
		"resource":      {credential.Resource},
	}
	if len(credential.Scopes) > 0 {
		form.Set("scope", strings.Join(credential.Scopes, " "))
	}
	response, err := s.requestToken(ctx, credential.TokenEndpoint, credential.TokenAuthMethod, credential.ClientID, secret, form)
	if err != nil {
		return "", fmt.Errorf("%w: refresh access token: %v", ErrLoginRequired, err)
	}
	updated, err := applyTokenResponse(credential, response)
	if err != nil {
		return "", err
	}
	if response.RefreshToken == "" {
		updated.RefreshToken = credential.RefreshToken
	}
	if response.Scope == "" {
		updated.Scopes = append([]string(nil), credential.Scopes...)
	}
	if err := s.Put(updated); err != nil {
		return "", err
	}
	return updated.AccessToken, nil
}

func (s *Store) requestToken(ctx context.Context, endpoint, authMethod, clientID, clientSecret string, form url.Values) (tokenResponse, error) {
	if endpoint == "" {
		return tokenResponse{}, errors.New("token endpoint is required")
	}
	requestForm := cloneValues(form)
	method := normalizeTokenAuthMethod(authMethod, clientSecret)
	switch method {
	case "client_secret_basic":
	case "client_secret_post":
		requestForm.Set("client_id", clientID)
		requestForm.Set("client_secret", clientSecret)
	case "none":
		requestForm.Set("client_id", clientID)
	default:
		return tokenResponse{}, fmt.Errorf("unsupported token auth method: %s", method)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(requestForm.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if method == "client_secret_basic" {
		request.SetBasicAuth(clientID, clientSecret)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var value tokenResponse
	if err := decoder.Decode(&value); err != nil {
		return tokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || value.Error != "" {
		message := value.Error
		if message == "" {
			message = response.Status
		}
		if value.Description != "" {
			message += ": " + value.Description
		}
		return tokenResponse{}, errors.New(message)
	}
	if value.AccessToken == "" {
		return tokenResponse{}, errors.New("token response is missing access_token")
	}
	return value, nil
}

func applyTokenResponse(credential Credential, response tokenResponse) (Credential, error) {
	credential.AccessToken = response.AccessToken
	credential.TokenType = response.TokenType
	credential.RefreshToken = response.RefreshToken
	credential.ExpiresAt = time.Time{}
	if response.ExpiresIn != "" {
		seconds, err := response.ExpiresIn.Int64()
		if err != nil || seconds < 0 {
			return Credential{}, errors.New("token response has invalid expires_in")
		}
		if seconds > 0 {
			credential.ExpiresAt = time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		}
	}
	if response.Scope != "" {
		credential.Scopes = strings.Fields(response.Scope)
	}
	return credential, nil
}

func credentialSecret(credential Credential) (string, error) {
	if credential.ClientSecret != "" {
		return credential.ClientSecret, nil
	}
	if credential.ClientSecretEnvVar == "" {
		return "", nil
	}
	value := strings.TrimSpace(os.Getenv(credential.ClientSecretEnvVar))
	if value == "" {
		return "", fmt.Errorf("%w: missing OAuth client secret environment variable %s", ErrLoginRequired, credential.ClientSecretEnvVar)
	}
	return value, nil
}

func normalizeTokenAuthMethod(method, secret string) string {
	switch method {
	case "client_secret_basic", "client_secret_post", "none":
		return method
	}
	if secret != "" {
		return "client_secret_basic"
	}
	return "none"
}

func cloneValues(value url.Values) url.Values {
	result := make(url.Values, len(value))
	for key, items := range value {
		result[key] = append([]string(nil), items...)
	}
	return result
}
