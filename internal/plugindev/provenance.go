package plugindev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const provenanceSchema = 1
const provenanceFile = "local-dev.json"

type Provenance struct {
	Schema            int       `json:"schema"`
	Origin            string    `json:"origin"`
	RepoRoot          string    `json:"repo_root"`
	PluginID          string    `json:"plugin_id"`
	SourceFingerprint string    `json:"source_fingerprint,omitempty"`
	ArtifactDigest    string    `json:"artifact_digest,omitempty"`
	BuiltAt           time.Time `json:"built_at,omitempty"`
}

func (p Provenance) Validate() error {
	if p.Schema != provenanceSchema || p.Origin != plugin.RegistryLocalDev || p.PluginID == "" || p.RepoRoot == "" {
		return fmt.Errorf("invalid local-dev provenance")
	}
	if _, err := verifiedRoot(p.RepoRoot); err != nil {
		return fmt.Errorf("local-dev provenance repo: %w", err)
	}
	return nil
}

func WriteProvenance(installed plugin.InstalledPlugin, provenance Provenance) error {
	if err := provenance.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(filepath.Join(installed.Root, provenanceFile), append(data, '\n'), 0o600)
}

func ReadProvenance(installed plugin.InstalledPlugin) (Provenance, error) {
	data, err := os.ReadFile(filepath.Join(installed.Root, provenanceFile))
	if err != nil {
		return Provenance{}, err
	}
	var provenance Provenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return Provenance{}, err
	}
	if err := provenance.Validate(); err != nil {
		return Provenance{}, err
	}
	return provenance, nil
}

func LocalDevTrust(publisher string) plugin.ActivationTrust {
	return plugin.ActivationTrust{Registry: plugin.RegistryLocalDev, Publisher: publisher, Trusted: true}
}
