package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func (s *Store) clientForTargets(trustedOrigins ...string) *http.Client {
	client := *s.client
	previous := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := validateOutboundURL(request.Context(), request.URL.String(), trustedOrigins...); err != nil {
			return fmt.Errorf("OAuth redirect denied: %w", err)
		}
		if previous != nil {
			return previous(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func validateOutboundURL(ctx context.Context, raw string, trustedOrigins ...string) error {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || (value.Scheme != "http" && value.Scheme != "https") || value.User != nil {
		return fmt.Errorf("invalid HTTP URL: %q", raw)
	}
	for _, trusted := range trustedOrigins {
		if sameOrigin(value, trusted) {
			return nil
		}
	}
	if value.Scheme != "https" {
		return errors.New("server-advertised OAuth URLs outside the configured server origin must use HTTPS")
	}
	host := value.Hostname()
	if strings.EqualFold(host, "localhost") {
		return errors.New("server-advertised OAuth URL resolves to localhost")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !publicOAuthIP(ip) {
			return fmt.Errorf("server-advertised OAuth URL uses non-public address %s", ip)
		}
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve OAuth host %s: %w", host, err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("OAuth host %s resolved to no addresses", host)
	}
	for _, address := range addresses {
		if !publicOAuthIP(address.IP) {
			return fmt.Errorf("server-advertised OAuth host %s resolves to non-public address %s", host, address.IP)
		}
	}
	return nil
}

func sameOrigin(value *url.URL, raw string) bool {
	trusted, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || trusted.Host == "" {
		return false
	}
	return strings.EqualFold(value.Scheme, trusted.Scheme) && strings.EqualFold(value.Hostname(), trusted.Hostname()) && effectivePort(value) == effectivePort(trusted)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if value.Scheme == "https" {
		return "443"
	}
	if value.Scheme == "http" {
		return "80"
	}
	return ""
}

func publicOAuthIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}
