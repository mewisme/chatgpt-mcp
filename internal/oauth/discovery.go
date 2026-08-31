package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type Discovery struct {
	Resource        string
	Issuer          string
	RequestedScopes []string
	ResourceMeta    *oauthex.ProtectedResourceMetadata
	AuthServerMeta  *oauthex.AuthServerMeta
}

type metadataCandidate struct {
	URL      string
	Resource string
}

func (s *Store) ProbeWWWAuthenticate(ctx context.Context, serverURL string) ([]string, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "server/discover",
		"params": map[string]any{"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "chatgpt-mcp", "version": "1.0.0"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}},
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "server/discover")
	response, err := s.clientForTargets(serverURL).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return append([]string(nil), response.Header.Values("WWW-Authenticate")...), nil
}

func (s *Store) Discover(ctx context.Context, serverURL, issuerPreference string, challengeHeaders []string) (*Discovery, error) {
	challenges, err := oauthex.ParseWWWAuthenticate(challengeHeaders)
	if err != nil {
		return nil, fmt.Errorf("parse WWW-Authenticate: %w", err)
	}
	candidates, err := protectedResourceCandidates(serverURL, challengeResourceMetadata(challenges))
	if err != nil {
		return nil, err
	}
	var resourceMeta *oauthex.ProtectedResourceMetadata
	var lastErr error
	for _, candidate := range candidates {
		if err := validateOutboundURL(ctx, candidate.URL, serverURL); err != nil {
			lastErr = err
			continue
		}
		value, err := oauthex.GetProtectedResourceMetadata(ctx, candidate.URL, candidate.Resource, s.clientForTargets(serverURL))
		if err != nil {
			lastErr = err
			continue
		}
		if value != nil {
			resourceMeta = value
			break
		}
	}
	if resourceMeta == nil {
		if lastErr != nil {
			return nil, fmt.Errorf("protected resource metadata discovery failed: %w", lastErr)
		}
		return nil, errors.New("protected resource metadata not found")
	}
	if len(resourceMeta.AuthorizationServers) == 0 {
		return nil, errors.New("protected resource metadata has no authorization_servers")
	}
	issuer := resourceMeta.AuthorizationServers[0]
	if strings.TrimSpace(issuerPreference) != "" {
		issuer = strings.TrimSpace(issuerPreference)
		if !slices.Contains(resourceMeta.AuthorizationServers, issuer) {
			return nil, fmt.Errorf("configured issuer %q is not advertised by the protected resource", issuer)
		}
	}
	if err := validateOutboundURL(ctx, issuer, serverURL); err != nil {
		return nil, fmt.Errorf("authorization server URL denied: %w", err)
	}
	authMeta, err := mcpauth.GetAuthServerMetadata(ctx, issuer, s.clientForTargets(serverURL, issuer))
	if err != nil {
		return nil, fmt.Errorf("authorization server metadata discovery failed: %w", err)
	}
	if authMeta == nil {
		return nil, errors.New("authorization server metadata not found")
	}
	if !slices.Contains(authMeta.CodeChallengeMethodsSupported, "S256") {
		return nil, errors.New("authorization server does not advertise PKCE S256")
	}
	for name, endpoint := range map[string]string{
		"authorization": authMeta.AuthorizationEndpoint,
		"token":         authMeta.TokenEndpoint,
		"registration":  authMeta.RegistrationEndpoint,
	} {
		if endpoint == "" {
			continue
		}
		if err := validateOutboundURL(ctx, endpoint, serverURL, issuer); err != nil {
			return nil, fmt.Errorf("%s endpoint denied: %w", name, err)
		}
	}
	scopes := challengeScopes(challenges)
	if len(scopes) == 0 {
		scopes = append([]string(nil), resourceMeta.ScopesSupported...)
	}
	return &Discovery{
		Resource: resourceMeta.Resource, Issuer: issuer, RequestedScopes: scopes,
		ResourceMeta: resourceMeta, AuthServerMeta: authMeta,
	}, nil
}

func protectedResourceCandidates(serverURL, challenged string) ([]metadataCandidate, error) {
	resource, err := url.Parse(serverURL)
	if err != nil || resource.Scheme == "" || resource.Host == "" {
		return nil, fmt.Errorf("invalid MCP server URL: %s", serverURL)
	}
	resource.Fragment = ""
	serverResource := resource.String()
	result := make([]metadataCandidate, 0, 3)
	seen := map[string]bool{}
	add := func(metadataURL, expectedResource string) {
		key := metadataURL + "\x00" + expectedResource
		if metadataURL == "" || seen[key] {
			return
		}
		seen[key] = true
		result = append(result, metadataCandidate{URL: metadataURL, Resource: expectedResource})
	}
	add(challenged, serverResource)
	pathMeta := *resource
	pathMeta.RawQuery = ""
	pathMeta.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(resource.Path, "/")
	add(pathMeta.String(), serverResource)
	rootResource := *resource
	rootResource.Path = ""
	rootResource.RawPath = ""
	rootResource.RawQuery = ""
	rootResource.Fragment = ""
	rootMeta := rootResource
	rootMeta.Path = "/.well-known/oauth-protected-resource"
	add(rootMeta.String(), rootResource.String())
	return result, nil
}

func challengeResourceMetadata(challenges []oauthex.Challenge) string {
	for _, challenge := range challenges {
		if value := challenge.Params["resource_metadata"]; value != "" {
			return value
		}
	}
	return ""
}

func challengeScopes(challenges []oauthex.Challenge) []string {
	for _, challenge := range challenges {
		if challenge.Scheme == "bearer" {
			return strings.Fields(challenge.Params["scope"])
		}
	}
	return nil
}
