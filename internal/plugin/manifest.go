package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/update"
)

const ManifestSchema = 1

type PluginID string
type Version string
type Capability string
type Permission string
type PluginType string

const (
	PermissionProcessExecute       Permission = "process/execute"
	PermissionNetworkOutbound      Permission = "network/outbound"
	PermissionFilesystemPluginData Permission = "filesystem/plugin-data"
	PermissionWorkspaceRead        Permission = "filesystem/workspace-read"
	PermissionWorkspaceWrite       Permission = "filesystem/workspace-write"
	PermissionHookToolObserve      Permission = "hook/tool-observe"
	PermissionHookToolControl      Permission = "hook/tool-control"
)

type Manifest struct {
	Schema       int                         `json:"schema"`
	ID           PluginID                    `json:"id"`
	Name         string                      `json:"name"`
	Publisher    string                      `json:"publisher"`
	Version      Version                     `json:"version"`
	Type         PluginType                  `json:"type"`
	Requires     Requirements                `json:"requires,omitempty"`
	Provides     []Capability                `json:"provides"`
	Permissions  []Permission                `json:"permissions"`
	Dependencies Dependencies                `json:"dependencies,omitempty"`
	Platforms    map[string]PlatformArtifact `json:"platforms"`
}

type Requirements struct {
	ChatGPTMCP string `json:"chatgpt-mcp,omitempty"`
}

type Dependencies struct {
	Capabilities []Capability `json:"capabilities,omitempty"`
}

type PlatformArtifact struct {
	Artifact   string `json:"artifact"`
	SHA256     string `json:"sha256"`
	Archive    string `json:"archive"`
	Entrypoint string `json:"entrypoint"`
}

type Publisher struct {
	Name     string           `json:"name"`
	Source   string           `json:"source"`
	Trusted  bool             `json:"trusted"`
	Sigstore SigstoreIdentity `json:"sigstore"`
}

type SigstoreIdentity struct {
	Issuer     string `json:"issuer"`
	Repository string `json:"repository"`
}

type Registry struct {
	Name                  string            `json:"name"`
	URL                   string            `json:"url"`
	UnqualifiedResolution bool              `json:"unqualified_resolution,omitempty"`
	Trust                 *SigstoreIdentity `json:"trust,omitempty"`
}

var canonicalNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)

var knownPermissions = map[Permission]struct{}{
	PermissionProcessExecute: {}, PermissionNetworkOutbound: {}, PermissionFilesystemPluginData: {}, PermissionWorkspaceRead: {}, PermissionWorkspaceWrite: {}, PermissionHookToolObserve: {}, PermissionHookToolControl: {},
}

var knownPluginTypes = map[PluginType]struct{}{
	"runtime": {}, "command-wrapper": {}, "hook": {}, "tool-provider": {}, "secret-provider": {}, "formatter": {},
}

func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode plugin manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) Validate() error {
	if manifest.Schema != ManifestSchema {
		return fmt.Errorf("unsupported plugin manifest schema: %d", manifest.Schema)
	}
	if !validCanonicalName(string(manifest.ID)) {
		return fmt.Errorf("invalid plugin id: %q", manifest.ID)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("plugin name is required")
	}
	if !validCanonicalName(manifest.Publisher) {
		return fmt.Errorf("invalid plugin publisher: %q", manifest.Publisher)
	}
	if err := validateVersion(string(manifest.Version)); err != nil {
		return fmt.Errorf("invalid plugin version %q: %w", manifest.Version, err)
	}
	if _, ok := knownPluginTypes[manifest.Type]; !ok {
		return fmt.Errorf("unknown plugin type: %q", manifest.Type)
	}
	if err := validateCoreConstraint(manifest.Requires.ChatGPTMCP); err != nil {
		return err
	}
	if len(manifest.Provides) == 0 {
		return errors.New("plugin must provide at least one capability")
	}
	if err := validateCapabilities(manifest.Provides, "provided"); err != nil {
		return err
	}
	if err := validateCapabilities(manifest.Dependencies.Capabilities, "dependency"); err != nil {
		return err
	}
	seenPermissions := make(map[Permission]struct{}, len(manifest.Permissions))
	for _, permission := range manifest.Permissions {
		if _, ok := knownPermissions[permission]; !ok {
			return fmt.Errorf("unknown plugin permission: %q", permission)
		}
		if _, ok := seenPermissions[permission]; ok {
			return fmt.Errorf("duplicate plugin permission: %q", permission)
		}
		seenPermissions[permission] = struct{}{}
	}
	if len(manifest.Platforms) == 0 {
		return errors.New("plugin must declare at least one platform artifact")
	}
	platforms := make([]string, 0, len(manifest.Platforms))
	for platform := range manifest.Platforms {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	for _, platform := range platforms {
		if !validPlatform(platform) {
			return fmt.Errorf("invalid plugin platform: %q", platform)
		}
		if err := manifest.Platforms[platform].validate(platform); err != nil {
			return err
		}
	}
	return nil
}

func (manifest Manifest) Platform(goos, goarch string) (PlatformArtifact, error) {
	if err := manifest.Validate(); err != nil {
		return PlatformArtifact{}, err
	}
	key := strings.TrimSpace(goos) + "/" + strings.TrimSpace(goarch)
	artifact, ok := manifest.Platforms[key]
	if !ok {
		return PlatformArtifact{}, fmt.Errorf("plugin %s@%s does not support platform %s", manifest.ID, manifest.Version, key)
	}
	return artifact, nil
}

func (manifest Manifest) CompatibleWithCore(coreVersion string) (bool, error) {
	constraint := strings.TrimSpace(manifest.Requires.ChatGPTMCP)
	if constraint == "" {
		return true, nil
	}
	operator, required, err := parseCoreConstraint(constraint)
	if err != nil {
		return false, err
	}
	comparison, err := update.CompareVersions(coreVersion, required)
	if err != nil {
		return false, fmt.Errorf("compare core version: %w", err)
	}
	switch operator {
	case ">=":
		return comparison >= 0, nil
	case ">":
		return comparison > 0, nil
	case "<=":
		return comparison <= 0, nil
	case "<":
		return comparison < 0, nil
	case "=", "":
		return comparison == 0, nil
	default:
		return false, fmt.Errorf("unsupported core version constraint operator: %q", operator)
	}
}

func ManifestDigest(manifest Manifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (artifact PlatformArtifact) validate(platform string) error {
	if !safeAssetName(artifact.Artifact) {
		return fmt.Errorf("platform %s has unsafe artifact filename: %q", platform, artifact.Artifact)
	}
	if !validSHA256(artifact.SHA256) {
		return fmt.Errorf("platform %s has invalid sha256 digest", platform)
	}
	if artifact.Archive != "zip" && artifact.Archive != "tar.gz" {
		return fmt.Errorf("platform %s has unsupported archive type: %q", platform, artifact.Archive)
	}
	if !safeRelativePath(artifact.Entrypoint) {
		return fmt.Errorf("platform %s has unsafe entrypoint: %q", platform, artifact.Entrypoint)
	}
	return nil
}

func validateCapabilities(capabilities []Capability, kind string) error {
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("invalid %s capability: %q", kind, capability)
		}
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("duplicate %s capability: %q", kind, capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func validCapability(capability Capability) bool {
	value := string(capability)
	parts := strings.Split(value, "/")
	if len(parts) != 2 || !validCanonicalName(parts[1]) {
		return false
	}
	switch parts[0] {
	case "shell", "command-wrapper", "hook", "tool-provider", "secret-provider", "formatter":
		return true
	default:
		return false
	}
}

func validPlatform(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && validCanonicalName(parts[0]) && validCanonicalName(parts[1])
}

func validCanonicalName(value string) bool {
	return canonicalNamePattern.MatchString(value)
}

func validateVersion(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\\+`) || strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		return update.ErrInvalidVersion
	}
	normalized, err := update.NormalizeVersion(value)
	if err != nil {
		return err
	}
	if strings.TrimPrefix(normalized, "v") != value {
		return update.ErrInvalidVersion
	}
	return nil
}

func validateCoreConstraint(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, _, err := parseCoreConstraint(value)
	return err
}

func parseCoreConstraint(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	operator := ""
	for _, candidate := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(value, candidate) {
			operator = candidate
			value = strings.TrimSpace(strings.TrimPrefix(value, candidate))
			break
		}
	}
	if strings.ContainsAny(value, " <>=,") {
		return "", "", fmt.Errorf("unsupported chatgpt-mcp version constraint: %q", value)
	}
	if err := validateVersion(value); err != nil {
		return "", "", fmt.Errorf("invalid chatgpt-mcp version constraint: %q", value)
	}
	return operator, value, nil
}

func safeAssetName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value == path.Base(value) && value != "." && value != ".." && !strings.ContainsAny(value, `\\:`)
}

func safeRelativePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, `\\`) || strings.Contains(value, ":") || strings.HasPrefix(value, "/") {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
