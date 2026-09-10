package outboundpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	AllowPrivate   bool
	TrustedOrigins []string
	LookupIPAddr   func(ctx context.Context, host string) ([]net.IPAddr, error)
}

func IsPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func SameOrigin(value *url.URL, raw string) bool {
	trusted, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || trusted.Host == "" {
		return false
	}
	return strings.EqualFold(value.Scheme, trusted.Scheme) && strings.EqualFold(value.Hostname(), trusted.Hostname()) && EffectivePort(value) == EffectivePort(trusted)
}

func EffectivePort(value *url.URL) string {
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

func ParseHTTPURL(raw string) (*url.URL, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || (value.Scheme != "http" && value.Scheme != "https") || value.User != nil {
		return nil, fmt.Errorf("invalid HTTP URL: %q", raw)
	}
	return value, nil
}

func ValidateURL(ctx context.Context, raw string, opts Options) error {
	value, err := ParseHTTPURL(raw)
	if err != nil {
		return err
	}
	for _, trusted := range opts.TrustedOrigins {
		if SameOrigin(value, trusted) {
			return nil
		}
	}
	if len(opts.TrustedOrigins) > 0 {
		if value.Scheme != "https" {
			return errors.New("server-advertised OAuth URLs outside the configured server origin must use HTTPS")
		}
		return validateResolvedHost(ctx, value.Hostname(), opts, true, "server-advertised OAuth")
	}

	host := value.Hostname()
	loopback := isLoopbackHost(host)
	if opts.AllowPrivate {
		if value.Scheme == "http" && !loopback {
			return errors.New("upstream URL must use HTTPS unless the host is loopback")
		}
		if loopback || isPrivateOrLinkLocalHost(ctx, opts, host) {
			return nil
		}
		return validateResolvedHost(ctx, host, opts, false, "upstream")
	}

	if value.Scheme != "https" {
		return errors.New("upstream URL must use HTTPS unless the host is loopback")
	}
	if loopback {
		return errors.New("upstream URL targets loopback; set allow_private_network to permit")
	}
	return validateResolvedHost(ctx, host, opts, true, "upstream")
}

func validateResolvedHost(ctx context.Context, host string, opts Options, rejectNonPublic bool, label string) error {
	if ip := net.ParseIP(host); ip != nil {
		if rejectNonPublic && !IsPublicIP(ip) {
			return fmt.Errorf("%s URL uses non-public address %s", label, ip)
		}
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		if rejectNonPublic {
			return fmt.Errorf("%s URL resolves to localhost", label)
		}
		return nil
	}
	addresses, err := lookupIPAddr(ctx, opts, host)
	if err != nil {
		return fmt.Errorf("resolve %s host %s: %w", label, host, err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("%s host %s resolved to no addresses", label, host)
	}
	if !rejectNonPublic {
		return nil
	}
	for _, address := range addresses {
		if !IsPublicIP(address.IP) {
			return fmt.Errorf("%s host %s resolves to non-public address %s", label, host, address.IP)
		}
	}
	return nil
}

func isPrivateOrLinkLocalHost(ctx context.Context, opts Options, host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return !IsPublicIP(ip)
	}
	addresses, err := lookupIPAddr(ctx, opts, host)
	if err != nil || len(addresses) == 0 {
		return false
	}
	for _, address := range addresses {
		if !IsPublicIP(address.IP) {
			return true
		}
	}
	return false
}

func lookupIPAddr(ctx context.Context, opts Options, host string) ([]net.IPAddr, error) {
	if opts.LookupIPAddr != nil {
		return opts.LookupIPAddr(ctx, host)
	}
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateDialAddr(network, address string, opts Options) error {
	if opts.AllowPrivate {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("dial address is not an IP: %s", address)
	}
	if !IsPublicIP(ip) {
		return fmt.Errorf("refusing dial to non-public address %s", ip)
	}
	_ = network
	return nil
}

// NewHTTPClient builds a client with redirect/dial SSRF checks. Timeout stays 0 for long-lived MCP streams.
func NewHTTPClient(opts Options) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if err := validateDialAddr(network, address, opts); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, address)
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if err := ValidateURL(request.Context(), request.URL.String(), opts); err != nil {
				return fmt.Errorf("redirect denied: %w", err)
			}
			return nil
		},
	}
}
