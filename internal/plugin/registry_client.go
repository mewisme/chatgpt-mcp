package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

const (
	maxRegistryMetadataSize int64 = 4 << 20
	DefaultRegistryCacheTTL       = 24 * time.Hour
)

type RegistrySignatureVerifier func(context.Context, []byte, []byte, SigstoreIdentity) error

type RegistryClient struct {
	HTTPClient *http.Client
	Layout     Layout
	UserAgent  string
	Verifier   RegistrySignatureVerifier
	Now        func() time.Time
}

type registryCacheMetadata struct {
	Schema    int       `json:"schema"`
	FetchedAt time.Time `json:"fetched_at"`
	URL       string    `json:"url"`
}

func (client RegistryClient) Refresh(ctx context.Context, registry Registry) (RegistrySnapshot, error) {
	if err := client.Layout.Validate(); err != nil {
		return RegistrySnapshot{}, err
	}
	if err := validateRegistryDescriptor(registry); err != nil {
		return RegistrySnapshot{}, err
	}
	indexData, indexSignature, err := client.fetchSigned(ctx, registry.URL, "index.json")
	if err != nil {
		return RegistrySnapshot{}, err
	}
	publishersData, publishersSignature, err := client.fetchSigned(ctx, registry.URL, "publishers.json")
	if err != nil {
		return RegistrySnapshot{}, err
	}
	identity, err := registryTrustIdentity(registry)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	verifier := client.Verifier
	if verifier == nil {
		verifier = VerifySignedBlob
	}
	if err := verifier(ctx, publishersData, publishersSignature, identity); err != nil {
		return RegistrySnapshot{}, fmt.Errorf("verify publisher registry signature: %w", err)
	}
	publishers, err := ParsePublisherIndex(publishersData)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	if err := verifier(ctx, indexData, indexSignature, identity); err != nil {
		return RegistrySnapshot{}, fmt.Errorf("verify plugin registry signature: %w", err)
	}
	index, err := ParseRegistryIndex(indexData)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	now := time.Now().UTC()
	if client.Now != nil {
		now = client.Now().UTC()
	}
	snapshot := RegistrySnapshot{Registry: registry, Index: index, Publishers: publishers, FetchedAt: now}
	if err := snapshot.Validate(); err != nil {
		return RegistrySnapshot{}, err
	}
	if err := client.writeCache(registry, indexData, indexSignature, publishersData, publishersSignature, now); err != nil {
		return RegistrySnapshot{}, err
	}
	return snapshot, nil
}

func (client RegistryClient) LoadCached(registry Registry, maxAge time.Duration) (RegistrySnapshot, error) {
	if err := client.Layout.Validate(); err != nil {
		return RegistrySnapshot{}, err
	}
	if err := validateRegistryDescriptor(registry); err != nil {
		return RegistrySnapshot{}, err
	}
	dir := client.cacheDir(registry)
	metadataData, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return RegistrySnapshot{}, err
	}
	var metadata registryCacheMetadata
	if err := decodeStrictJSON(metadataData, &metadata); err != nil {
		return RegistrySnapshot{}, fmt.Errorf("decode plugin registry cache metadata: %w", err)
	}
	if metadata.Schema != 1 || metadata.URL != registry.URL || metadata.FetchedAt.IsZero() {
		return RegistrySnapshot{}, errors.New("plugin registry cache metadata is invalid")
	}
	now := time.Now().UTC()
	if client.Now != nil {
		now = client.Now().UTC()
	}
	if maxAge > 0 && (now.Before(metadata.FetchedAt) || now.Sub(metadata.FetchedAt) > maxAge) {
		return RegistrySnapshot{}, errors.New("plugin registry cache is stale")
	}
	indexData, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return RegistrySnapshot{}, err
	}
	publishersData, err := os.ReadFile(filepath.Join(dir, "publishers.json"))
	if err != nil {
		return RegistrySnapshot{}, err
	}
	index, err := ParseRegistryIndex(indexData)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	publishers, err := ParsePublisherIndex(publishersData)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	snapshot := RegistrySnapshot{Registry: registry, Index: index, Publishers: publishers, FetchedAt: metadata.FetchedAt}
	if err := snapshot.Validate(); err != nil {
		return RegistrySnapshot{}, err
	}
	return snapshot, nil
}

func (client RegistryClient) FetchManifest(ctx context.Context, resolved ResolvedPlugin) (Manifest, []byte, error) {
	data, signature, err := client.fetchSigned(ctx, resolved.Registry.URL, resolved.ManifestName)
	if err != nil {
		return Manifest{}, nil, err
	}
	verifier := client.Verifier
	if verifier == nil {
		verifier = VerifySignedBlob
	}
	if !resolved.Publisher.Trusted {
		return Manifest{}, nil, fmt.Errorf("publisher %s is not trusted", resolved.Publisher.Name)
	}
	if err := verifier(ctx, data, signature, resolved.Publisher.Sigstore); err != nil {
		return Manifest{}, nil, fmt.Errorf("verify plugin manifest signature: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return Manifest{}, nil, err
	}
	if manifest.ID != resolved.PluginID || manifest.Version != resolved.Version || manifest.Publisher != resolved.Publisher.Name {
		return Manifest{}, nil, errors.New("resolved plugin manifest identity mismatch")
	}
	return manifest, signature, nil
}

func (client RegistryClient) fetchSigned(ctx context.Context, baseURL, asset string) ([]byte, []byte, error) {
	data, err := client.fetch(ctx, baseURL, asset, maxRegistryMetadataSize)
	if err != nil {
		return nil, nil, err
	}
	signature, err := client.fetch(ctx, baseURL, asset+".sigstore.json", maxRegistryMetadataSize)
	if err != nil {
		return nil, nil, err
	}
	return data, signature, nil
}

func (client RegistryClient) fetch(ctx context.Context, baseURL, asset string, limit int64) ([]byte, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/")
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("plugin registry URL must use HTTPS: %s", baseURL)
	}
	if !safeRegistryAssetName(asset) {
		return nil, fmt.Errorf("unsafe plugin registry asset name: %q", asset)
	}
	resolved, err := base.Parse(url.PathEscape(asset))
	if err != nil || resolved.Host != base.Host {
		return nil, errors.New("plugin registry asset URL escaped registry host")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.String(), nil)
	if err != nil {
		return nil, err
	}
	userAgent := strings.TrimSpace(client.UserAgent)
	if userAgent == "" {
		userAgent = "chatgpt-mcp/plugin-registry"
	}
	request.Header.Set("User-Agent", userAgent)
	httpClient := securePluginHTTPClient(client.HTTPClient, 30*time.Second, base.Host)
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin registry returned %s for %s", response.Status, asset)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("plugin registry asset %s exceeds size limit", asset)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit || len(data) == 0 {
		return nil, fmt.Errorf("plugin registry asset %s has invalid size", asset)
	}
	return data, nil
}

func (client RegistryClient) writeCache(registry Registry, index, indexSig, publishers, publishersSig []byte, fetchedAt time.Time) error {
	dir := client.cacheDir(registry)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	files := map[string][]byte{"index.json": index, "index.json.sigstore.json": indexSig, "publishers.json": publishers, "publishers.json.sigstore.json": publishersSig}
	for name, data := range files {
		if err := state.WriteFileAtomic(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	metadata := registryCacheMetadata{Schema: 1, FetchedAt: fetchedAt, URL: registry.URL}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(filepath.Join(dir, "metadata.json"), append(data, '\n'), 0600)
}

func (client RegistryClient) cacheDir(registry Registry) string {
	return filepath.Join(client.Layout.CacheRoot, "plugins", "registries", registry.Name)
}

func validateRegistryDescriptor(registry Registry) error {
	if !validCanonicalName(registry.Name) {
		return fmt.Errorf("invalid plugin registry name: %q", registry.Name)
	}
	parsed, err := url.Parse(strings.TrimSpace(registry.URL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("plugin registry URL must use HTTPS: %s", registry.URL)
	}
	if registry.Name != OfficialRegistryName {
		if registry.Trust == nil || strings.TrimSpace(registry.Trust.Issuer) == "" || strings.TrimSpace(registry.Trust.Repository) == "" {
			return fmt.Errorf("plugin registry %s requires a pinned Sigstore trust identity", registry.Name)
		}
	}
	return nil
}

func registryTrustIdentity(registry Registry) (SigstoreIdentity, error) {
	if registry.Name == OfficialRegistryName {
		return SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}, nil
	}
	if registry.Trust == nil || strings.TrimSpace(registry.Trust.Issuer) == "" || strings.TrimSpace(registry.Trust.Repository) == "" {
		return SigstoreIdentity{}, errors.New("plugin registry trust identity is required")
	}
	return *registry.Trust, nil
}

func safeRegistryAssetName(asset string) bool {
	asset = strings.TrimSuffix(asset, ".sigstore.json")
	return safeAssetName(asset)
}
