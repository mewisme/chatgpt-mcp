package update

import (
	"context"
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const githubActionsOIDCIssuer = "https://token.actions.githubusercontent.com"

func VerifyChecksumSignature(ctx context.Context, checksumPath, signaturePath, version string) error {
	version, err := NormalizeVersion(version)
	if err != nil {
		return err
	}
	signedBundle, err := bundle.LoadJSONFromPath(signaturePath)
	if err != nil {
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
	identity, err := verify.NewShortCertificateIdentity(githubActionsOIDCIssuer, "", releaseWorkflowIdentity(version), "")
	if err != nil {
		return fmt.Errorf("create release signing identity: %w", err)
	}
	checksum, err := os.Open(checksumPath)
	if err != nil {
		return err
	}
	defer checksum.Close()
	if _, err := verifier.Verify(signedBundle, verify.NewPolicy(verify.WithArtifact(checksum), verify.WithCertificateIdentity(identity))); err != nil {
		return fmt.Errorf("sigstore verification failed: %w", err)
	}
	return nil
}

func releaseWorkflowIdentity(version string) string {
	return fmt.Sprintf("https://github.com/%s/%s/.github/workflows/release.yml@refs/tags/%s", DefaultOwner, DefaultRepo, version)
}
