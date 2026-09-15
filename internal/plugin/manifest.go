package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	CapabilityWebUIAdmin Capability = "web-ui/admin"

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
	Artifact   string              `json:"artifact,omitempty"`
	SHA256     string              `json:"sha256,omitempty"`
	Archive    string              `json:"archive,omitempty"`
	Entrypoint string              `json:"entrypoint,omitempty"`
	Host       *HostExecutableSpec `json:"host,omitempty"`
}

type HostExecutableSpec struct {
	Executable     string                `json:"executable"`
	Checks         []HostExecutableCheck `json:"checks,omitempty"`
	Install        []HostInstallHint     `json:"install,omitempty"`
	Portable       *HostPortableInstall  `json:"portable,omitempty"`
	CommandWrapper *HostCommandWrapper   `json:"command_wrapper,omitempty"`
}

type HostExecutableCheck struct {
	Name             string   `json:"name,omitempty"`
	Args             []string `json:"args,omitempty"`
	SuccessExitCodes []int    `json:"success_exit_codes,omitempty"`
	StdoutPrefix     string   `json:"stdout_prefix,omitempty"`
	StdoutContains   string   `json:"stdout_contains,omitempty"`
}

type HostInstallHint struct {
	Label      string   `json:"label,omitempty"`
	Command    string   `json:"command,omitempty"`
	Executable string   `json:"executable,omitempty"`
	Args       []string `json:"args,omitempty"`
}

type HostPortableInstall struct {
	URL           string `json:"url"`
	ChecksumURL   string `json:"checksum_url"`
	ChecksumAsset string `json:"checksum_asset"`
	Archive       string `json:"archive"`
	Entrypoint    string `json:"entrypoint"`
}

type HostCommandWrapper struct {
	Args                 []string `json:"args"`
	RewriteExitCodes     []int    `json:"rewrite_exit_codes"`
	PassthroughExitCodes []int    `json:"passthrough_exit_codes,omitempty"`
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
	"runtime": {}, "command-wrapper": {}, "hook": {}, "tool-provider": {}, "secret-provider": {}, "formatter": {}, "web-ui": {},
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
	if err := validateHookPermissions(manifest.Provides, seenPermissions); err != nil {
		return err
	}
	if err := validateWrapperPermissions(manifest.Provides, seenPermissions); err != nil {
		return err
	}
	webUICapabilities := 0
	for _, capability := range manifest.Provides {
		if strings.HasPrefix(string(capability), "web-ui/") {
			webUICapabilities++
		}
	}
	if manifest.Type == "web-ui" {
		if len(manifest.Provides) != 1 || webUICapabilities != 1 {
			return errors.New("web-ui plugin requires exactly one web-ui capability")
		}
		if len(manifest.Permissions) != 0 {
			return errors.New("web-ui plugin cannot request runtime permissions")
		}
	} else if webUICapabilities > 0 {
		return errors.New("web-ui capabilities require plugin type web-ui")
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
		artifact := manifest.Platforms[platform]
		if err := artifact.validate(platform); err != nil {
			return err
		}
		if manifest.Type == "web-ui" && !strings.HasSuffix(strings.ToLower(artifact.Entrypoint), ".html") {
			return fmt.Errorf("platform %s web-ui entrypoint must be an HTML file", platform)
		}
		if artifact.HostBacked() {
			if manifest.Type != "command-wrapper" {
				return fmt.Errorf("platform %s host executable is only supported for command-wrapper plugins", platform)
			}
			if len(manifest.Provides) != 1 || !strings.HasPrefix(string(manifest.Provides[0]), commandWrapperPrefix) {
				return fmt.Errorf("platform %s host executable requires exactly one command-wrapper capability", platform)
			}
			wrapper := strings.TrimPrefix(string(manifest.Provides[0]), commandWrapperPrefix)
			executable := strings.TrimSuffix(strings.ToLower(artifact.Host.Executable), ".exe")
			if executable != wrapper {
				return fmt.Errorf("platform %s host executable %q does not match command-wrapper capability %q", platform, artifact.Host.Executable, manifest.Provides[0])
			}
			if artifact.Host.CommandWrapper == nil {
				return fmt.Errorf("platform %s host command-wrapper configuration is required", platform)
			}
		}
	}
	return nil
}

func validateHookPermissions(capabilities []Capability, permissions map[Permission]struct{}) error {
	for _, capability := range capabilities {
		var required Permission
		switch capability {
		case CapabilityHookPreToolUse:
			required = PermissionHookToolControl
		case CapabilityHookPostToolUse, CapabilityHookToolError, CapabilityHookToolDenied:
			required = PermissionHookToolObserve
		default:
			continue
		}
		if _, ok := permissions[PermissionProcessExecute]; !ok {
			return fmt.Errorf("hook capability %s requires permission %s", capability, PermissionProcessExecute)
		}
		if _, ok := permissions[required]; !ok {
			return fmt.Errorf("hook capability %s requires permission %s", capability, required)
		}
	}
	return nil
}

func validateWrapperPermissions(capabilities []Capability, permissions map[Permission]struct{}) error {
	for _, capability := range capabilities {
		if !strings.HasPrefix(string(capability), commandWrapperPrefix) {
			continue
		}
		if _, ok := permissions[PermissionProcessExecute]; !ok {
			return fmt.Errorf("command wrapper capability %s requires permission %s", capability, PermissionProcessExecute)
		}
	}
	return nil
}

func (manifest Manifest) Platform(goos, goarch string) (PlatformArtifact, error) {
	if err := manifest.Validate(); err != nil {
		return PlatformArtifact{}, err
	}
	key := strings.TrimSpace(goos) + "/" + strings.TrimSpace(goarch)
	if artifact, ok := manifest.Platforms[key]; ok {
		return artifact, nil
	}
	if artifact, ok := manifest.Platforms["any/any"]; ok {
		return artifact, nil
	}
	return PlatformArtifact{}, fmt.Errorf("plugin %s@%s does not support platform %s", manifest.ID, manifest.Version, key)
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
	if artifact.Host != nil {
		if artifact.Artifact != "" || artifact.SHA256 != "" || artifact.Archive != "" || artifact.Entrypoint != "" {
			return fmt.Errorf("platform %s cannot mix host executable and packaged artifact fields", platform)
		}
		if !safeHostExecutable(artifact.Host.Executable) {
			return fmt.Errorf("platform %s has invalid host executable: %q", platform, artifact.Host.Executable)
		}
		for index, check := range artifact.Host.Checks {
			if err := validateHostCheck(check); err != nil {
				return fmt.Errorf("platform %s host check %d: %w", platform, index+1, err)
			}
		}
		for index, hint := range artifact.Host.Install {
			if err := validateHostInstallHint(hint); err != nil {
				return fmt.Errorf("platform %s host install hint %d: %w", platform, index+1, err)
			}
		}
		if artifact.Host.Portable != nil {
			if err := validateHostPortable(*artifact.Host.Portable); err != nil {
				return fmt.Errorf("platform %s host portable install: %w", platform, err)
			}
		}
		if artifact.Host.CommandWrapper != nil {
			if err := validateHostCommandWrapper(*artifact.Host.CommandWrapper); err != nil {
				return fmt.Errorf("platform %s host command wrapper: %w", platform, err)
			}
		}
		return nil
	}
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

func (artifact PlatformArtifact) HostBacked() bool {
	return artifact.Host != nil
}

func validateHostCheck(check HostExecutableCheck) error {
	if strings.TrimSpace(check.Name) == "" && len(check.Args) == 0 && strings.TrimSpace(check.StdoutPrefix) == "" && strings.TrimSpace(check.StdoutContains) == "" {
		return errors.New("empty check")
	}
	for _, arg := range check.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("check argument contains NUL")
		}
	}
	return validateExitCodes(check.SuccessExitCodes, true)
}

func validateHostInstallHint(hint HostInstallHint) error {
	command, executable := strings.TrimSpace(hint.Command), strings.TrimSpace(hint.Executable)
	if command == "" && executable == "" {
		return errors.New("command or executable is required")
	}
	if command != "" && strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("command contains invalid characters")
	}
	if executable != "" && !safeHostExecutable(executable) {
		return fmt.Errorf("invalid executable %q", executable)
	}
	for _, arg := range hint.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("argument contains NUL")
		}
	}
	if executable == "" && len(hint.Args) > 0 {
		return errors.New("args require executable")
	}
	return nil
}

func validateHostPortable(portable HostPortableInstall) error {
	if !validHTTPSURL(portable.URL) || !validHTTPSURL(portable.ChecksumURL) {
		return errors.New("portable URLs must use HTTPS")
	}
	if !safeAssetName(portable.ChecksumAsset) {
		return fmt.Errorf("invalid checksum asset %q", portable.ChecksumAsset)
	}
	parsed, _ := url.Parse(portable.URL)
	if path.Base(parsed.Path) != portable.ChecksumAsset {
		return fmt.Errorf("portable URL asset %q does not match checksum asset %q", path.Base(parsed.Path), portable.ChecksumAsset)
	}
	if portable.Archive != "zip" && portable.Archive != "tar.gz" {
		return fmt.Errorf("unsupported archive type %q", portable.Archive)
	}
	if !safeRelativePath(portable.Entrypoint) {
		return fmt.Errorf("unsafe entrypoint %q", portable.Entrypoint)
	}
	return nil
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func (hint HostInstallHint) Runnable() bool {
	return safeHostExecutable(strings.TrimSpace(hint.Executable))
}

func (hint HostInstallHint) DisplayCommand() string {
	executable := strings.TrimSpace(hint.Executable)
	if safeHostExecutable(executable) {
		parts := append([]string{executable}, hint.Args...)
		return strings.Join(parts, " ")
	}
	return strings.TrimSpace(hint.Command)
}

func validateHostCommandWrapper(wrapper HostCommandWrapper) error {
	if len(wrapper.Args) == 0 {
		return errors.New("args are required")
	}
	commandArgs := 0
	for _, arg := range wrapper.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("argument contains NUL")
		}
		if arg == "{command}" {
			commandArgs++
		}
	}
	if commandArgs != 1 {
		return errors.New("args must contain exactly one {command} placeholder")
	}
	if err := validateExitCodes(wrapper.RewriteExitCodes, false); err != nil {
		return fmt.Errorf("rewrite exit codes: %w", err)
	}
	if err := validateExitCodes(wrapper.PassthroughExitCodes, true); err != nil {
		return fmt.Errorf("passthrough exit codes: %w", err)
	}
	for _, code := range wrapper.RewriteExitCodes {
		if containsExitCode(wrapper.PassthroughExitCodes, code) {
			return fmt.Errorf("exit code %d cannot be both rewrite and passthrough", code)
		}
	}
	return nil
}

func validateExitCodes(codes []int, allowEmpty bool) error {
	if len(codes) == 0 {
		if allowEmpty {
			return nil
		}
		return errors.New("at least one exit code is required")
	}
	seen := map[int]struct{}{}
	for _, code := range codes {
		if code < 0 || code > 255 {
			return fmt.Errorf("invalid exit code %d", code)
		}
		if _, ok := seen[code]; ok {
			return fmt.Errorf("duplicate exit code %d", code)
		}
		seen[code] = struct{}{}
	}
	return nil
}

func containsExitCode(codes []int, target int) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}

func safeHostExecutable(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value == path.Base(value) && value != "." && value != ".." && !strings.ContainsAny(value, `/\:`)
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
	case "shell", "command-wrapper", "hook", "tool-provider", "secret-provider", "formatter", "web-ui":
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
