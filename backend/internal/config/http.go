package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HTTPConfig controls the management HTTP server and security policies.
type HTTPConfig struct {
	MetricsToken            string        `config:"metrics_token"`
	Address                 string        `config:"address"`
	AllowedOrigins          []string      `config:"allowed_origins"`
	TrustedProxies          []string      `config:"trusted_proxies"`
	MaxConcurrentQueries    int           `config:"max_concurrent_queries"`
	MaxConcurrentLogQueries int           `config:"max_concurrent_log_queries"`
	RequestTimeout          time.Duration `config:"request_timeout"`
	ReadHeaderTimeout       time.Duration `config:"read_header_timeout"`
	ReadTimeout             time.Duration `config:"read_timeout"`
	WriteTimeout            time.Duration `config:"write_timeout"`
	IdleTimeout             time.Duration `config:"idle_timeout"`
	MaxHeaderBytes          int           `config:"max_header_bytes"`
	ManagementMaxBodyBytes  int64         `config:"management_max_body_bytes"`
	RateLimitRequests       int           `config:"rate_limit_requests"`
	RateLimitWindow         time.Duration `config:"rate_limit_window"`
	RateLimitAuthRequests   int           `config:"rate_limit_auth_requests"`
	RateLimitReplayRequests int           `config:"rate_limit_replay_requests"`
}

// WithRequestDefaults fills request limits when the HTTP server is constructed with a partial config.
func (http HTTPConfig) WithRequestDefaults() HTTPConfig {
	defaults := defaultConfig().HTTP
	if http.MaxConcurrentQueries <= 0 {
		http.MaxConcurrentQueries = defaults.MaxConcurrentQueries
	}
	if http.MaxConcurrentLogQueries <= 0 {
		http.MaxConcurrentLogQueries = defaults.MaxConcurrentLogQueries
	}
	if http.RequestTimeout <= 0 {
		http.RequestTimeout = defaults.RequestTimeout
	}
	if http.RateLimitRequests <= 0 {
		http.RateLimitRequests = defaults.RateLimitRequests
	}
	if http.RateLimitAuthRequests <= 0 {
		http.RateLimitAuthRequests = defaults.RateLimitAuthRequests
	}
	if http.RateLimitReplayRequests <= 0 {
		http.RateLimitReplayRequests = defaults.RateLimitReplayRequests
	}
	return http
}

func (configuration Config) validateHTTP(v *validator) {
	http := configuration.HTTP
	http.validateServer(v)
	http.validateLimits(v)
	http.validateSecurity(v)
}

func (http HTTPConfig) validateServer(v *validator) {
	if err := validateListenAddress(http.Address); err != nil {
		v.add("http.address", err.Error())
	}
	if http.ReadHeaderTimeout <= 0 {
		v.add("http.read_header_timeout", "must be positive")
	}
	if http.ReadTimeout <= 0 {
		v.add("http.read_timeout", "must be positive")
	}
	if http.WriteTimeout <= 0 {
		v.add("http.write_timeout", "must be positive")
	}
	if http.IdleTimeout <= 0 {
		v.add("http.idle_timeout", "must be positive")
	}
	if http.RequestTimeout <= 0 || http.RequestTimeout >= http.WriteTimeout {
		v.add("http.request_timeout", "must be positive and shorter than write_timeout")
	}
	if http.ReadTimeout > 0 && http.ReadHeaderTimeout > http.ReadTimeout {
		v.add("http.read_header_timeout", "must not exceed read_timeout")
	}
}

func (http HTTPConfig) validateLimits(v *validator) {
	if http.MaxConcurrentQueries <= 0 {
		v.add("http.max_concurrent_queries", "must be positive")
	}
	if http.MaxConcurrentLogQueries <= 0 {
		v.add("http.max_concurrent_log_queries", "must be positive")
	}
	if http.MaxHeaderBytes <= 0 {
		v.add("http.max_header_bytes", "must be positive")
	}
	if http.ManagementMaxBodyBytes <= 0 {
		v.add("http.management_max_body_bytes", "must be positive")
	}
	if http.RateLimitRequests <= 0 {
		v.add("http.rate_limit_requests", "must be positive")
	}
	if http.RateLimitAuthRequests <= 0 {
		v.add("http.rate_limit_auth_requests", "must be positive")
	}
	if http.RateLimitReplayRequests <= 0 {
		v.add("http.rate_limit_replay_requests", "must be positive")
	}
	if http.RateLimitWindow <= 0 {
		v.add("http.rate_limit_window", "must be positive")
	}
}

func (http HTTPConfig) validateSecurity(v *validator) {
	if http.MetricsToken != "" && len(http.MetricsToken) < 32 {
		v.add("http.metrics_token", "must contain at least 32 bytes")
	}

	for _, proxy := range http.TrustedProxies {
		if _, err := ParseProxyPrefix(proxy); err != nil {
			v.add("http.trusted_proxies", fmt.Sprintf("invalid proxy %q: %v", proxy, err))
		}
	}

	for _, origin := range http.AllowedOrigins {
		if err := validateOrigin(origin); err != nil {
			v.add("http.allowed_origins", fmt.Sprintf("invalid origin %q: %v", origin, err))
		}
	}
}

// validateListenAddress validates a TCP listen address.
func validateListenAddress(address string) error {
	if strings.TrimSpace(address) != address {
		return fmt.Errorf("must not contain surrounding whitespace")
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("must use host:port format: %w", err)
	}

	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if strings.ContainsAny(host, "/ \t\r\n") {
		return fmt.Errorf("invalid listen host")
	}

	return nil
}

// validateOrigin accepts only a scheme and an authority.
func validateOrigin(origin string) error {
	if origin == "" || strings.TrimSpace(origin) != origin {
		return fmt.Errorf("origin must not be empty or contain surrounding whitespace")
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return err
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("invalid origin host")
	}
	if parsed.Opaque != "" || parsed.Path != "" || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		parsed.RawFragment != "" {
		return fmt.Errorf("origin must not contain path, query or fragment")
	}
	if strings.ContainsAny(parsed.Host, " \t\r\n") {
		return fmt.Errorf("invalid origin host")
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid origin port")
		}
	}

	return nil
}

// ParseProxyPrefix parses a CIDR or a single IP address.
func ParseProxyPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)

	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		if prefix.Addr().Is4In6() {
			return netip.Prefix{}, fmt.Errorf("IPv4-mapped IPv6 CIDR is not supported")
		}
		if prefix.Addr().Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("IPv6 zone is not supported")
		}

		return prefix.Masked(), nil
	}

	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if address.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("IPv6 zone is not supported")
	}

	address = address.Unmap()
	return netip.PrefixFrom(address, address.BitLen()), nil
}

// TrustedProxyPrefixes returns validated and normalized proxy ranges.
func (http HTTPConfig) TrustedProxyPrefixes() []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(http.TrustedProxies))

	for _, entry := range http.TrustedProxies {
		if prefix, err := ParseProxyPrefix(entry); err == nil {
			prefixes = append(prefixes, prefix)
		}
	}

	return prefixes
}

func ClientAddress(remoteAddr string, forwarded []string, trusted []netip.Prefix) string {
	_, peer, ok := splitPeer(remoteAddr)
	if !ok {
		return "unknown"
	}
	if !containsAddress(trusted, peer) {
		return peer.String()
	}

	for headerIndex := len(forwarded) - 1; headerIndex >= 0; headerIndex-- {
		hops := strings.Split(forwarded[headerIndex], ",")

		for index := len(hops) - 1; index >= 0; index-- {
			hop, err := netip.ParseAddr(strings.TrimSpace(hops[index]))
			if err != nil || hop.Zone() != "" {
				return peer.String()
			}

			hop = hop.Unmap()
			if !containsAddress(trusted, hop) {
				return hop.String()
			}
		}
	}

	return peer.String()
}

// FromTrustedProxy reports whether the TCP peer is trusted.
func FromTrustedProxy(remoteAddr string, trusted []netip.Prefix) bool {
	_, peer, ok := splitPeer(remoteAddr)
	return ok && containsAddress(trusted, peer)
}

// splitPeer parses a host:port or bare IP address.
func splitPeer(remoteAddr string) (string, netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}

	address, err := netip.ParseAddr(host)
	if err != nil || address.Zone() != "" {
		return host, netip.Addr{}, false
	}

	return host, address.Unmap(), true
}

func containsAddress(prefixes []netip.Prefix, address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}

	address = address.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
