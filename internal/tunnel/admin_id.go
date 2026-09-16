package tunnel

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const maxAdminProfileIDLen = 64

var adminProfileIDPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9])?$`)

func ValidateAdminProfileID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("admin profile id is required")
	}
	if len(value) > maxAdminProfileIDLen {
		return fmt.Errorf("admin profile id must be at most %d characters", maxAdminProfileIDLen)
	}
	if !adminProfileIDPattern.MatchString(value) {
		return errors.New("admin profile id may contain only letters, numbers, '.', '_', and '-'")
	}
	return nil
}

func ValidateControlPlaneBaseURL(value string) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid OpenAI tunnel control plane base URL %q", raw)
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("OpenAI tunnel control plane base URL must use HTTPS unless the host is loopback")
	}
	return nil
}
