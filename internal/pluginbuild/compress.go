package pluginbuild

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func compressBinary(path, goos string) error {
	if !ShouldCompress(goos) {
		return nil
	}
	if upxDisabled(os.Getenv(EnvUPX)) {
		return nil
	}
	upx, err := exec.LookPath("upx")
	if err != nil {
		return fmt.Errorf("upx is required to compress native plugin binaries; install UPX %s or set %s=%s for local builds", PinnedUPXVersion, EnvUPX, EnvUPXOff)
	}
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	cmd := exec.Command(upx, "--best", path)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upx --best: %w", err)
	}
	test := exec.Command(upx, "-t", path)
	test.Stdout = os.Stderr
	test.Stderr = os.Stderr
	if err := test.Run(); err != nil {
		return fmt.Errorf("upx -t: %w", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "upx %s %s: %d -> %d bytes (%s)\n", goos, path, before.Size(), after.Size(), upxVersion(upx))
	return nil
}

func upxVersion(upx string) string {
	out, err := exec.Command(upx, "--version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
