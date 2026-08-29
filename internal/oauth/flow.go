package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const defaultFlowTTL = 10 * time.Minute

type FlowSession struct {
	ID               string    `json:"session_id"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type FlowManager struct {
	mu       sync.Mutex
	store    *Store
	sessions map[string]pendingLogin
	ttl      time.Duration
}

type pendingLogin struct {
	Config       LoginConfig
	Discovery    *Discovery
	Registration registration
	Scopes       []string
	State        string
	Verifier     string
	RedirectURL  string
	ExpiresAt    time.Time
}

type registration struct {
	Kind               string
	ClientID           string
	ClientSecret       string
	ClientSecretEnvVar string
	ClientMetadataURL  string
	TokenAuthMethod    string
}

func NewFlowManager(store *Store) *FlowManager {
	if store == nil {
		store = NewStore(Path())
	}
	return &FlowManager{store: store, sessions: map[string]pendingLogin{}, ttl: defaultFlowTTL}
}

func (m *FlowManager) Begin(ctx context.Context, config LoginConfig, redirectBase, extraScope string) (FlowSession, error) {
	config.ServerID = strings.TrimSpace(config.ServerID)
	config.ServerURL = strings.TrimSpace(config.ServerURL)
	if config.ServerID == "" || config.ServerURL == "" {
		return FlowSession{}, errors.New("OAuth login requires server id and URL")
	}
	if err := validateRedirectURL(redirectBase); err != nil {
		return FlowSession{}, err
	}
	challengeHeaders, err := m.store.ProbeWWWAuthenticate(ctx, config.ServerURL)
	if err != nil {
		return FlowSession{}, fmt.Errorf("probe MCP authorization challenge: %w", err)
	}
	discovery, err := m.store.Discover(ctx, config.ServerURL, config.Issuer, challengeHeaders)
	if err != nil {
		return FlowSession{}, err
	}
	previous, _ := m.store.Get(config.ServerID)
	scopes := unionScopes(discovery.RequestedScopes, strings.Fields(config.Scope), strings.Fields(extraScope))
	if previous.ServerURL == config.ServerURL && previous.Issuer == discovery.Issuer {
		scopes = unionScopes(scopes, previous.Scopes)
	}
	if slices.Contains(discovery.AuthServerMeta.ScopesSupported, "offline_access") && !slices.Contains(scopes, "offline_access") {
		scopes = append(scopes, "offline_access")
	}
	id, err := randomToken(24)
	if err != nil {
		return FlowSession{}, err
	}
	redirectURL, err := flowRedirectURL(redirectBase, id)
	if err != nil {
		return FlowSession{}, err
	}
	registration, err := m.store.resolveRegistration(ctx, config, discovery, redirectURL, scopes)
	if err != nil {
		return FlowSession{}, err
	}
	state, err := randomToken(32)
	if err != nil {
		return FlowSession{}, err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return FlowSession{}, err
	}
	authorizationURL, err := buildAuthorizationURL(discovery.AuthServerMeta.AuthorizationEndpoint, registration.ClientID, redirectURL, discovery.Resource, state, pkceChallenge(verifier), scopes)
	if err != nil {
		return FlowSession{}, err
	}
	expiresAt := time.Now().UTC().Add(m.ttl)
	m.mu.Lock()
	m.cleanupLocked(time.Now())
	m.sessions[id] = pendingLogin{
		Config: config, Discovery: discovery, Registration: registration, Scopes: scopes, State: state, Verifier: verifier, RedirectURL: redirectURL, ExpiresAt: expiresAt,
	}
	m.mu.Unlock()
	return FlowSession{ID: id, AuthorizationURL: authorizationURL, ExpiresAt: expiresAt}, nil
}

func (m *FlowManager) Complete(ctx context.Context, id, state, code, issuer, oauthError, errorDescription string) (Credential, error) {
	m.mu.Lock()
	m.cleanupLocked(time.Now())
	pending, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return Credential{}, errors.New("OAuth login session not found or expired")
	}
	if state != pending.State {
		m.mu.Unlock()
		return Credential{}, errors.New("OAuth state mismatch")
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	if oauthError != "" {
		if errorDescription != "" {
			oauthError += ": " + errorDescription
		}
		return Credential{}, errors.New(oauthError)
	}
	if code == "" {
		return Credential{}, errors.New("OAuth callback is missing authorization code")
	}
	if err := ValidateIssuerResponse(issuer, pending.Discovery.Issuer, pending.Discovery.AuthServerMeta.AuthorizationResponseIssParameterSupported); err != nil {
		return Credential{}, err
	}
	secret, err := registrationSecret(pending.Registration)
	if err != nil {
		return Credential{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {pending.RedirectURL},
		"code_verifier": {pending.Verifier},
		"resource":      {pending.Discovery.Resource},
	}
	token, err := m.store.requestToken(ctx, pending.Discovery.AuthServerMeta.TokenEndpoint, pending.Registration.TokenAuthMethod, pending.Registration.ClientID, secret, form)
	if err != nil {
		return Credential{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	credential := Credential{
		ServerID: pending.Config.ServerID, ServerURL: pending.Config.ServerURL, Resource: pending.Discovery.Resource, Issuer: pending.Discovery.Issuer,
		Registration: pending.Registration.Kind, ClientID: pending.Registration.ClientID, ClientSecret: pending.Registration.ClientSecret,
		ClientSecretEnvVar: pending.Registration.ClientSecretEnvVar, ClientMetadataURL: pending.Registration.ClientMetadataURL,
		AuthorizationURL: pending.Discovery.AuthServerMeta.AuthorizationEndpoint, TokenEndpoint: pending.Discovery.AuthServerMeta.TokenEndpoint,
		TokenAuthMethod: pending.Registration.TokenAuthMethod, Scopes: append([]string(nil), pending.Scopes...),
	}
	credential, err = applyTokenResponse(credential, token)
	if err != nil {
		return Credential{}, err
	}
	if token.Scope == "" {
		credential.Scopes = append([]string(nil), pending.Scopes...)
	}
	if err := m.store.Put(credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (m *FlowManager) Cancel(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *FlowManager) cleanupLocked(now time.Time) {
	for id, session := range m.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
}

func (s *Store) resolveRegistration(ctx context.Context, config LoginConfig, discovery *Discovery, redirectURL string, scopes []string) (registration, error) {
	metadataURL := strings.TrimSpace(config.ClientMetadataURL)
	if metadataURL != "" && discovery.AuthServerMeta.ClientIDMetadataDocumentSupported {
		if err := validateClientMetadataURL(metadataURL); err != nil {
			return registration{}, err
		}
		return registration{Kind: "cimd", ClientID: metadataURL, ClientMetadataURL: metadataURL, TokenAuthMethod: "none"}, nil
	}
	clientID := strings.TrimSpace(config.ClientID)
	if clientID != "" {
		secret := ""
		if env := strings.TrimSpace(config.ClientSecretEnvVar); env != "" {
			secret = strings.TrimSpace(os.Getenv(env))
			if secret == "" {
				return registration{}, fmt.Errorf("missing OAuth client secret environment variable: %s", env)
			}
		}
		return registration{
			Kind: "preregistered", ClientID: clientID, ClientSecretEnvVar: strings.TrimSpace(config.ClientSecretEnvVar),
			TokenAuthMethod: selectTokenAuthMethod(discovery.AuthServerMeta.TokenEndpointAuthMethodsSupported, secret),
		}, nil
	}
	if discovery.AuthServerMeta.RegistrationEndpoint == "" {
		if metadataURL != "" {
			return registration{}, errors.New("authorization server does not support configured Client ID Metadata Document and has no DCR endpoint")
		}
		return registration{}, errors.New("OAuth client registration is required: configure a client ID or Client ID Metadata Document")
	}
	grantTypes := []string{"authorization_code"}
	if slices.Contains(discovery.AuthServerMeta.GrantTypesSupported, "refresh_token") || slices.Contains(discovery.AuthServerMeta.ScopesSupported, "offline_access") {
		grantTypes = append(grantTypes, "refresh_token")
	}
	response, err := oauthex.RegisterClient(ctx, discovery.AuthServerMeta.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{
		RedirectURIs: []string{redirectURL}, TokenEndpointAuthMethod: "none", GrantTypes: grantTypes,
		ResponseTypes: []string{"code"}, ClientName: "chatgpt-mcp", Scope: strings.Join(scopes, " "), ApplicationType: "native",
	}, s.client)
	if err != nil {
		return registration{}, fmt.Errorf("dynamic client registration: %w", err)
	}
	method := response.TokenEndpointAuthMethod
	if method == "" {
		method = normalizeTokenAuthMethod("", response.ClientSecret)
	}
	return registration{Kind: "dcr", ClientID: response.ClientID, ClientSecret: response.ClientSecret, TokenAuthMethod: method}, nil
}

func ValidateIssuerResponse(iss, expected string, advertised bool) error {
	if advertised && iss == "" {
		return errors.New("authorization response is missing required iss parameter")
	}
	if iss != "" && iss != expected {
		return fmt.Errorf("authorization response issuer %q does not match expected issuer %q", iss, expected)
	}
	return nil
}

func ValidateRedirectOrigin(raw string) (string, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || (value.Scheme != "http" && value.Scheme != "https") {
		return "", errors.New("redirect origin must be an absolute HTTP or HTTPS origin")
	}
	if value.User != nil || value.RawQuery != "" || value.Fragment != "" || (value.Path != "" && value.Path != "/") {
		return "", errors.New("redirect origin must not contain path, query, fragment, or user info")
	}
	if value.Scheme == "http" && !isLoopbackHost(value.Hostname()) {
		return "", errors.New("HTTP OAuth callback origins must use a loopback host; use HTTPS for remote admin access")
	}
	return value.Scheme + "://" + value.Host, nil
}

func validateRedirectURL(raw string) error {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || (value.Scheme != "http" && value.Scheme != "https") {
		return errors.New("OAuth redirect URL must be absolute HTTP or HTTPS")
	}
	if value.Scheme == "http" && !isLoopbackHost(value.Hostname()) {
		return errors.New("HTTP OAuth redirect URLs must use a loopback host")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func flowRedirectURL(base, id string) (string, error) {
	value, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	value.Path = strings.TrimRight(value.Path, "/") + "/" + url.PathEscape(id)
	value.RawQuery = ""
	value.Fragment = ""
	return value.String(), nil
}

func buildAuthorizationURL(endpoint, clientID, redirectURL, resource, state, challenge string, scopes []string) (string, error) {
	value, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := value.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("resource", resource)
	if len(scopes) > 0 {
		query.Set("scope", strings.Join(scopes, " "))
	}
	value.RawQuery = query.Encode()
	return value.String(), nil
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func unionScopes(groups ...[]string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, group := range groups {
		for _, scope := range group {
			scope = strings.TrimSpace(scope)
			if scope == "" || seen[scope] {
				continue
			}
			seen[scope] = true
			result = append(result, scope)
		}
	}
	return result
}

func validateClientMetadataURL(raw string) error {
	value, err := url.Parse(raw)
	if err != nil || value.Scheme != "https" || value.Host == "" || value.Path == "" || value.Path == "/" {
		return errors.New("Client ID Metadata Document URL must be a non-root HTTPS URL")
	}
	return nil
}

func registrationSecret(value registration) (string, error) {
	if value.ClientSecret != "" {
		return value.ClientSecret, nil
	}
	if value.ClientSecretEnvVar == "" {
		return "", nil
	}
	secret := strings.TrimSpace(os.Getenv(value.ClientSecretEnvVar))
	if secret == "" {
		return "", fmt.Errorf("missing OAuth client secret environment variable: %s", value.ClientSecretEnvVar)
	}
	return secret, nil
}

func selectTokenAuthMethod(supported []string, secret string) string {
	if secret == "" {
		return "none"
	}
	if slices.Contains(supported, "client_secret_post") {
		return "client_secret_post"
	}
	if slices.Contains(supported, "client_secret_basic") || len(supported) == 0 {
		return "client_secret_basic"
	}
	return "client_secret_basic"
}
