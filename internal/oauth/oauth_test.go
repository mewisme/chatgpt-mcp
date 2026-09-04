package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStoreRoundTripAndStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oauth.json")
	store := NewStore(path)
	expires := time.Now().UTC().Add(time.Hour)
	if err := store.Put(Credential{ServerID: "alpha", ServerURL: "https://example.test/mcp", Issuer: "https://issuer.test", AccessToken: "secret", RefreshToken: "refresh", ExpiresAt: expires, Scopes: []string{"read"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"secret"`) || strings.Contains(string(data), `"refresh"`) || !strings.Contains(string(data), "secret-file") {
		t.Fatalf("oauth file leaked credential value or missed secret-file marker: %s", data)
	}
	credential, err := store.Get("alpha")
	if err != nil || credential.AccessToken != "secret" || credential.RefreshToken != "refresh" {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	status, err := store.Status("alpha")
	if err != nil || !status.Configured || !status.HasRefreshToken || status.ClientID != "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%#o want 0600", info.Mode().Perm())
		}
	}
	if err := store.Delete("alpha"); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status("alpha")
	if err != nil || status.Configured {
		t.Fatalf("status after delete=%#v err=%v", status, err)
	}
}

func TestValidateIssuerResponseFinalRules(t *testing.T) {
	issuer := "https://issuer.example"
	cases := []struct {
		iss        string
		advertised bool
		wantErr    bool
	}{
		{issuer, true, false},
		{"", true, true},
		{issuer, false, false},
		{"", false, false},
		{"https://evil.example", true, true},
		{"https://evil.example", false, true},
	}
	for _, test := range cases {
		if err := ValidateIssuerResponse(test.iss, issuer, test.advertised); (err != nil) != test.wantErr {
			t.Fatalf("iss=%q advertised=%t err=%v", test.iss, test.advertised, err)
		}
	}
}

func TestLoginUsesDiscoveryPKCEDCRAndResource(t *testing.T) {
	var server *httptest.Server
	var dcrApplicationType, tokenResource, tokenVerifier string
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		resource := server.URL + "/mcp"
		issuer := server.URL + "/issuer"
		switch request.URL.Path {
		case "/mcp":
			writer.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+server.URL+`/.well-known/oauth-protected-resource/mcp", scope="read"`)
			writer.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"resource": resource, "authorization_servers": []string{issuer}, "scopes_supported": []string{"read"}})
		case "/.well-known/oauth-authorization-server/issuer":
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token",
				"registration_endpoint": server.URL + "/register", "response_types_supported": []string{"code"},
				"grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"},
				"code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"read", "offline_access"},
				"authorization_response_iss_parameter_supported": true,
			})
		case "/register":
			writer.Header().Set("Content-Type", "application/json")
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			dcrApplicationType, _ = body["application_type"].(string)
			redirects, _ := body["redirect_uris"].([]any)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"client_id": "dcr-client", "token_endpoint_auth_method": "none", "application_type": "native",
				"redirect_uris": redirects, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
			})
		case "/token":
			writer.Header().Set("Content-Type", "application/json")
			_ = request.ParseForm()
			tokenResource = request.Form.Get("resource")
			tokenVerifier = request.Form.Get("code_verifier")
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 3600, "scope": "read offline_access"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store := NewStoreWithClient(filepath.Join(t.TempDir(), "oauth.json"), server.Client())
	credential, err := store.Login(context.Background(), LoginConfig{ServerID: "alpha", ServerURL: server.URL + "/mcp"}, LoginOptions{OnURL: func(raw string) error {
		authURL, err := url.Parse(raw)
		if err != nil {
			return err
		}
		query := authURL.Query()
		if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" || query.Get("resource") != server.URL+"/mcp" {
			t.Fatalf("authorization URL=%s", raw)
		}
		callback := query.Get("redirect_uri") + "?code=code&state=" + url.QueryEscape(query.Get("state")) + "&iss=" + url.QueryEscape(server.URL+"/issuer")
		response, err := http.Get(callback)
		if err == nil {
			_ = response.Body.Close()
		}
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if dcrApplicationType != "native" {
		t.Fatalf("application_type=%q", dcrApplicationType)
	}
	if tokenResource != server.URL+"/mcp" || tokenVerifier == "" {
		t.Fatalf("resource=%q verifier=%q", tokenResource, tokenVerifier)
	}
	if credential.AccessToken != "access" || credential.RefreshToken != "refresh" || credential.Registration != "dcr" {
		t.Fatalf("credential=%#v", credential)
	}
}

func TestAccessTokenRefreshIncludesResource(t *testing.T) {
	var resource string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		resource = request.Form.Get("resource")
		if request.Form.Get("grant_type") != "refresh_token" {
			t.Fatalf("grant_type=%q", request.Form.Get("grant_type"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600})
	}))
	defer server.Close()
	store := NewStoreWithClient(filepath.Join(t.TempDir(), "oauth.json"), server.Client())
	if err := store.Put(Credential{
		ServerID: "alpha", ServerURL: "https://resource.example/mcp", Resource: "https://resource.example/mcp",
		Issuer: server.URL, ClientID: "client", TokenEndpoint: server.URL, TokenAuthMethod: "none", AccessToken: "old", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	token, err := store.AccessToken(context.Background(), RuntimeConfig{ServerID: "alpha", ServerURL: "https://resource.example/mcp"})
	if err != nil || token != "new-access" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if resource != "https://resource.example/mcp" {
		t.Fatalf("resource=%q", resource)
	}
	updated, _ := store.Get("alpha")
	if updated.RefreshToken != "new-refresh" {
		t.Fatalf("refresh token was not rotated")
	}
}

func TestOAuthNetworkPolicyAllowsConfiguredOriginAndRejectsPrivatePivot(t *testing.T) {
	ctx := context.Background()
	configured := "http://127.0.0.1:37421/mcp"
	if err := validateOutboundURL(ctx, "http://127.0.0.1:37421/.well-known/oauth-protected-resource", configured); err != nil {
		t.Fatalf("configured local origin was rejected: %v", err)
	}
	for _, raw := range []string{
		"http://127.0.0.1:9999/metadata",
		"https://127.0.0.1/metadata",
		"https://169.254.169.254/latest/meta-data/",
		"https://10.0.0.1/oauth",
		"https://[::1]/oauth",
	} {
		if err := validateOutboundURL(ctx, raw, configured); err == nil {
			t.Fatalf("private OAuth pivot was accepted: %s", raw)
		}
	}
}

func TestOAuthClientRejectsRedirectToUntrustedPrivateOrigin(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/private", http.StatusFound)
	}))
	defer source.Close()
	store := NewStoreWithClient(filepath.Join(t.TempDir(), "oauth.json"), source.Client())
	request, err := http.NewRequest(http.MethodGet, source.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := store.clientForTargets(source.URL).Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "OAuth redirect denied") {
		t.Fatalf("private redirect was not denied: %v", err)
	}
}

func TestClientMetadataURLValidation(t *testing.T) {
	for _, raw := range []string{"http://example.com/client.json", "https://example.com/", "javascript:alert(1)"} {
		if err := validateClientMetadataURL(raw); err == nil {
			t.Fatalf("accepted invalid URL %q", raw)
		}
	}
	if err := validateClientMetadataURL("https://example.com/oauth/client.json"); err != nil {
		t.Fatal(err)
	}
}

func TestUnionScopesStable(t *testing.T) {
	got := strings.Join(unionScopes([]string{"a", "b"}, []string{"b", "c"}), " ")
	if got != "a b c" {
		t.Fatalf("got %q", got)
	}
}

func TestLegacyOAuthFileMigratesCredentialsToSecretFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oauth.json")
	legacy := diskStore{Version: storeVersion, Credentials: map[string]Credential{"alpha": {ServerID: "alpha", ClientID: "client", ClientSecret: "legacy-client-value", AccessToken: "legacy-access-value", RefreshToken: "legacy-refresh-value"}}}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	credential, err := store.Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if credential.ClientSecret != "legacy-client-value" || credential.AccessToken != "legacy-access-value" || credential.RefreshToken != "legacy-refresh-value" {
		t.Fatalf("credential=%#v", credential)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"legacy-client-value", "legacy-access-value", "legacy-refresh-value"} {
		if strings.Contains(string(migrated), value) {
			t.Fatalf("oauth file still contains legacy credential: %s", migrated)
		}
	}
	if strings.Count(string(migrated), "secret-file") < 3 {
		t.Fatalf("oauth file missing secret-file markers: %s", migrated)
	}
}
