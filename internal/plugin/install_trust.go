package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const installedTrustSchema = 1
const installedTrustFile = "install.json"

type installedTrustRecord struct {
	Schema         int    `json:"schema"`
	Registry       string `json:"registry"`
	Publisher      string `json:"publisher"`
	ManifestDigest string `json:"manifest_digest"`
	ArtifactDigest string `json:"artifact_digest"`
}

func (store *Store) writeInstalledTrust(installed InstalledPlugin, trust ActivationTrust) error {
	if !trust.Trusted {
		return errors.New("plugin publisher is not trusted")
	}
	if !validCanonicalName(trust.Registry) || !validCanonicalName(trust.Publisher) {
		return errors.New("plugin install trust metadata is invalid")
	}
	if installed.Manifest.Publisher != trust.Publisher {
		return fmt.Errorf("trusted publisher %q does not match manifest publisher %q", trust.Publisher, installed.Manifest.Publisher)
	}
	manifestDigest, err := ManifestDigest(installed.Manifest)
	if err != nil {
		return err
	}
	artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return err
	}
	record := installedTrustRecord{Schema: installedTrustSchema, Registry: trust.Registry, Publisher: trust.Publisher, ManifestDigest: manifestDigest, ArtifactDigest: platformLockDigest(artifact)}
	path := filepath.Join(installed.Root, installedTrustFile)
	if data, err := os.ReadFile(path); err == nil {
		var existing installedTrustRecord
		if decodeErr := decodeStrictJSON(data, &existing); decodeErr != nil {
			return fmt.Errorf("decode installed plugin trust metadata: %w", decodeErr)
		}
		if existing != record {
			return errors.New("installed plugin trust metadata does not match verified activation")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (store *Store) readInstalledTrust(installed InstalledPlugin) (installedTrustRecord, error) {
	data, err := os.ReadFile(filepath.Join(installed.Root, installedTrustFile))
	if err != nil {
		return installedTrustRecord{}, fmt.Errorf("read installed plugin trust metadata: %w", err)
	}
	var record installedTrustRecord
	if err := decodeStrictJSON(data, &record); err != nil {
		return installedTrustRecord{}, fmt.Errorf("decode installed plugin trust metadata: %w", err)
	}
	if record.Schema != installedTrustSchema || !validCanonicalName(record.Registry) || !validCanonicalName(record.Publisher) || !validPrefixedSHA256(record.ManifestDigest) || !validPrefixedSHA256(record.ArtifactDigest) {
		return installedTrustRecord{}, errors.New("installed plugin trust metadata is invalid")
	}
	manifestDigest, err := ManifestDigest(installed.Manifest)
	if err != nil {
		return installedTrustRecord{}, err
	}
	artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return installedTrustRecord{}, err
	}
	if installed.Manifest.Publisher != record.Publisher || manifestDigest != record.ManifestDigest || platformLockDigest(artifact) != record.ArtifactDigest {
		return installedTrustRecord{}, errors.New("installed plugin trust metadata integrity verification failed")
	}
	return record, nil
}

func (store *Store) verifyInstalledIntegrity(ctx context.Context, installed InstalledPlugin) error {
	artifact, err := installed.Manifest.Platform(store.runtime.OS, store.runtime.Arch)
	if err != nil {
		return err
	}
	if artifact.HostBacked() {
		return preflightHostPath(nonNilContext(ctx), installed.Entrypoint, artifact.Host)
	}
	return verifyPackagedPayload(installed)
}
