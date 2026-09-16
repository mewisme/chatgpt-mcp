package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

func VerifySignedBlob(ctx context.Context, artifact, signature []byte, identity SigstoreIdentity) error {
	if len(artifact) == 0 || len(signature) == 0 {
		return errors.New("signed plugin artifact and signature are required")
	}
	issuer := strings.TrimSpace(identity.Issuer)
	repository := strings.Trim(strings.TrimSpace(identity.Repository), "/")
	if issuer == "" || repository == "" {
		return errors.New("sigstore issuer and repository are required")
	}
	var signedBundle bundle.Bundle
	if err := json.Unmarshal(signature, &signedBundle); err != nil {
		return fmt.Errorf("load Sigstore bundle: %w", err)
	}
	trustedRoot, err := root.FetchTrustedRootWithOptions(tuf.DefaultOptions().WithContext(ctx))
	if err != nil {
		return fmt.Errorf("load Sigstore trusted root: %w", err)
	}
	verifier, err := verify.NewVerifier(trustedRoot, verify.WithSignedCertificateTimestamps(1), verify.WithObserverTimestamps(1), verify.WithTransparencyLog(1))
	if err != nil {
		return fmt.Errorf("create Sigstore verifier: %w", err)
	}
	identityPattern := fmt.Sprintf(`^https://github\.com/%s/\.github/workflows/plugins\.yml@refs/(heads/main|tags/plugins)$`, regexpEscapeRepository(repository))
	certificateIdentity, err := verify.NewShortCertificateIdentity(issuer, "", "", identityPattern)
	if err != nil {
		return fmt.Errorf("create plugin signing identity: %w", err)
	}
	if _, err := verifier.Verify(&signedBundle, verify.NewPolicy(verify.WithArtifact(bytes.NewReader(artifact)), verify.WithCertificateIdentity(certificateIdentity))); err != nil {
		return fmt.Errorf("sigstore verification failed: %w", err)
	}
	return nil
}

func regexpEscapeRepository(value string) string {
	replacer := strings.NewReplacer(`\\`, `\\\\`, `.`, `\.`, `+`, `\+`, `*`, `\*`, `?`, `\?`, `(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `^`, `\^`, `$`, `\$`, `|`, `\|`)
	return replacer.Replace(value)
}
