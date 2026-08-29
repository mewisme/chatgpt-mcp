package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type registration struct {
	Kind               string
	ClientID           string
	ClientSecret       string
	ClientSecretEnvVar string
	ClientMetadataURL  string
	TokenAuthMethod    string
}

type callbackResult struct {
	Code string
	Iss  string
	Err  error
}

func (s *Store) Login(ctx context.Context, config LoginConfig, options LoginOptions) (Credential, error) {
	config.ServerID = strings.TrimSpace(config.ServerID)
	config.ServerURL = strings.TrimSpace(config.ServerURL)
	if config.ServerID == "" || config.ServerURL == "" {
		return Credential{}, errors.New("OAuth login requires server id and URL")
	}
	challengeHeaders, err := s.ProbeWWWAuthenticate(ctx, config.ServerURL)
	if err != nil {
		return Credential{}, fmt.Errorf("probe MCP authorization challenge: %w", err)
	}
	discovery, err := s.Discover(ctx, config.ServerURL, config.Issuer, challengeHeaders)
	if err != nil {
		return Credential{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Credential{}, fmt.Errorf("open OAuth callback listener: %w", err)
	}
	defer listener.Close()
	redirectURL := "http://" + listener.Addr().String() + "/callback"

	previous, _ := s.Get(config.ServerID)
	scopes := unionScopes(discovery.RequestedScopes, strings.Fields(config.Scope), strings.Fields(options.ExtraScope))
	if previous.ServerURL == config.ServerURL && previous.Issuer == discovery.Issuer {
		scopes = unionScopes(scopes, previous.Scopes)
	}
	if slices.Contains(discovery.AuthServerMeta.ScopesSupported, "offline_access") && !slices.Contains(scopes, "offline_access") {
		scopes = append(scopes, "offline_access")
	}
	registration, err := s.resolveRegistration(ctx, config, discovery, redirectURL, scopes, previous)
	if err != nil {
		return Credential{}, err
	}
	state, err := randomToken(32)
	if err != nil {
		return Credential{}, err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return Credential{}, err
	}
	challenge := pkceChallenge(verifier)
	authorizationURL, err := buildAuthorizationURL(discovery.AuthServerMeta.AuthorizationEndpoint, registration.ClientID, redirectURL, discovery.Resource, state, challenge, scopes)
	if err != nil {
		return Credential{}, err
	}

	resultCh := make(chan callbackResult, 1)
	serverErrCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != state {
			http.Error(writer, "OAuth callback rejected", http.StatusBadRequest)
			return
		}
		if value := query.Get("error"); value != "" {
			description := query.Get("error_description")
			if description != "" {
				value += ": " + description
			}
			select {
			case resultCh <- callbackResult{Err: errors.New(value)}:
			default:
			}
			http.Error(writer, "OAuth authorization failed", http.StatusBadRequest)
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(writer, "OAuth callback rejected", http.StatusBadRequest)
			return
		}
		iss := query.Get("iss")
		if err := ValidateIssuerResponse(iss, discovery.Issuer, discovery.AuthServerMeta.AuthorizationResponseIssParameterSupported); err != nil {
			select {
			case resultCh <- callbackResult{Err: err}:
			default:
			}
			http.Error(writer, "OAuth callback rejected", http.StatusBadRequest)
			return
		}
		select {
		case resultCh <- callbackResult{Code: code, Iss: iss}:
		default:
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("Authorization received. You can close this window."))
	})
	callbackServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := callbackServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = callbackServer.Shutdown(shutdownCtx)
		cancel()
	}()
	if options.OnURL != nil {
		if err := options.OnURL(authorizationURL); err != nil {
			return Credential{}, err
		}
	}

	var callback callbackResult
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case err := <-serverErrCh:
		return Credential{}, fmt.Errorf("OAuth callback server: %w", err)
	case callback = <-resultCh:
	}
	if callback.Err != nil {
		return Credential{}, callback.Err
	}
	secret, err := registrationSecret(registration)
	if err != nil {
		return Credential{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {callback.Code},
		"redirect_uri":  {redirectURL},
		"code_verifier": {verifier},
		"resource":      {discovery.Resource},
	}
	token, err := s.requestToken(ctx, discovery.AuthServerMeta.TokenEndpoint, registration.TokenAuthMethod, registration.ClientID, secret, form)
	if err != nil {
		return Credential{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	credential := Credential{
		ServerID: config.ServerID, ServerURL: config.ServerURL, Resource: discovery.Resource, Issuer: discovery.Issuer,
		Registration: registration.Kind, ClientID: registration.ClientID, ClientSecret: registration.ClientSecret,
		ClientSecretEnvVar: registration.ClientSecretEnvVar, ClientMetadataURL: registration.ClientMetadataURL,
		AuthorizationURL: discovery.AuthServerMeta.AuthorizationEndpoint, TokenEndpoint: discovery.AuthServerMeta.TokenEndpoint,
		TokenAuthMethod: registration.TokenAuthMethod, Scopes: append([]string(nil), scopes...),
	}
	credential, err = applyTokenResponse(credential, token)
	if err != nil {
		return Credential{}, err
	}
	if token.Scope == "" {
		credential.Scopes = append([]string(nil), scopes...)
	}
	if err := s.Put(credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (s *Store) resolveRegistration(ctx context.Context, config LoginConfig, discovery *Discovery, redirectURL string, scopes []string, previous Credential) (registration, error) {
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
	if previous.ServerURL == config.ServerURL && previous.Issuer == discovery.Issuer && previous.Registration == "dcr" && previous.ClientID != "" {
		return registration{
			Kind: "dcr", ClientID: previous.ClientID, ClientSecret: previous.ClientSecret,
			TokenAuthMethod: previous.TokenAuthMethod,
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
