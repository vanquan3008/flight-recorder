package config

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// HTTPConfig controls the management HTTP server and security policies.
type HTTPConfig struct {
	// MetricsToken lets a metrics scraper read /metrics in production without a person's sign-in.
	MetricsToken   string   `config:"metrics_token"`
	Address        string   `config:"address"`
	AllowedOrigins []string `config:"allowed_origins"`
	TrustedProxies []string `config:"trusted_proxies"`

	// Limits concurrent management requests to protect the shared DB pool.
	MaxConcurrentQueries int `config:"max_concurrent_queries"`

	// RequestTimeout should be shorter than WriteTimeout.
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

func (configuration Config) validateHTTP(validator *validator) {
	if strings.TrimSpace(configuration.HTTP.Address) == "" {
		validator.add("http.address", "must not be empty")
	}
	if configuration.HTTP.ReadHeaderTimeout <= 0 {
		validator.add("http.read_header_timeout", "must be positive")
	}
	if configuration.HTTP.ReadTimeout <= 0 {
		validator.add("http.read_timeout", "must be positive")
	}
	if configuration.HTTP.WriteTimeout <= 0 {
		validator.add("http.write_timeout", "must be positive")
	}
	if configuration.HTTP.IdleTimeout <= 0 {
		validator.add("http.idle_timeout", "must be positive")
	}
	if configuration.HTTP.MaxHeaderBytes <= 0 {
		validator.add("http.max_header_bytes", "must be positive")
	}
	if configuration.HTTP.ManagementMaxBodyBytes <= 0 {
		validator.add("http.management_max_body_bytes", "must be positive")
	}
	if configuration.HTTP.MaxConcurrentQueries <= 0 {
		validator.add("http.max_concurrent_queries", "must be positive")
	}
	if configuration.HTTP.RequestTimeout <= 0 || configuration.HTTP.RequestTimeout >= configuration.HTTP.WriteTimeout {
		validator.add("http.request_timeout", "must be positive and shorter than http.write_timeout")
	}
	if configuration.HTTP.RateLimitRequests <= 0 {
		validator.add("http.rate_limit_requests", "must be positive")
	}
	if configuration.HTTP.RateLimitAuthRequests <= 0 {
		validator.add("http.rate_limit_auth_requests", "must be positive")
	}
	if configuration.HTTP.RateLimitReplayRequests <= 0 {
		validator.add("http.rate_limit_replay_requests", "must be positive")
	}
	if configuration.HTTP.RateLimitWindow <= 0 {
		validator.add("http.rate_limit_window", "must be positive")
	}
	if configuration.HTTP.MetricsToken != "" && len(configuration.HTTP.MetricsToken) < 32 {
		validator.add("http.metrics_token", "must contain at least 32 characters")
	}
	for _, proxy := range configuration.HTTP.TrustedProxies {
		if _, err := ParseProxyPrefix(proxy); err != nil {
			validator.add("http.trusted_proxies", "invalid entry: "+proxy+" (use CIDR or single IP)")
		}
	}
	for _, origin := range configuration.HTTP.AllowedOrigins {
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			validator.add("http.allowed_origins", "invalid origin: "+origin)
		}
	}
}

// ParseProxyPrefix reads a trusted proxy entry: a CIDR such as 10.0.0.0/8, or a
// single address, which is taken as a /32 (IPv4) or /128 (IPv6).
func ParseProxyPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "/") {
		return netip.ParsePrefix(value)
	}
	address, parseError := netip.ParseAddr(value)
	if parseError != nil {
		return netip.Prefix{}, parseError
	}
	return netip.PrefixFrom(address.Unmap(), address.Unmap().BitLen()), nil
}

// TrustedProxyPrefixes returns the configured trusted proxies. Validation
// rejects bad entries at startup; one that slips through is skipped, which
// fails closed because an unlisted address is never trusted.
func (configuration HTTPConfig) TrustedProxyPrefixes() []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(configuration.TrustedProxies))
	for _, entry := range configuration.TrustedProxies {
		if prefix, parseError := ParseProxyPrefix(entry); parseError == nil {
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return prefixes
}

// ClientAddress is the address a request is attributed to for rate limits and
// failure budgets. Forwarded headers are written by the client unless a proxy
// overwrites them, so X-Forwarded-For is read only when the TCP peer is one of
// the trusted proxies. It is read from the right: each hop appends the address
// it received from, so skipping trusted proxies leaves the first address a
// trusted party actually saw. A leftmost value is whatever the client chose to
// send and is never used. forwarded holds every X-Forwarded-For header value.
func ClientAddress(remoteAddr string, forwarded []string, trusted []netip.Prefix) string {
	host, peer, parsed := splitPeer(remoteAddr)
	if !parsed {
		if host == "" {
			return "unknown"
		}
		return host
	}
	if !containsAddress(trusted, peer) {
		return peer.String()
	}
	var hops []string
	for _, header := range forwarded {
		hops = append(hops, strings.Split(header, ",")...)
	}
	for index := len(hops) - 1; index >= 0; index-- {
		hop, hopError := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if hopError != nil {
			return peer.String() // a malformed chain is not evidence of anything
		}
		if hop = hop.Unmap(); !containsAddress(trusted, hop) {
			return hop.String()
		}
	}
	return peer.String()
}

// FromTrustedProxy reports whether the TCP peer at remoteAddr is one of the trusted proxies, the
// only peer whose forwarded headers are believed.
func FromTrustedProxy(remoteAddr string, trusted []netip.Prefix) bool {
	_, peer, parsed := splitPeer(remoteAddr)
	return parsed && containsAddress(trusted, peer)
}

// splitPeer reads the host of a connection's remote address. parsed is false when the host is not
// an IP address, and host is then returned as it was written.
func splitPeer(remoteAddr string) (host string, peer netip.Addr, parsed bool) {
	host, _, splitError := net.SplitHostPort(remoteAddr)
	if splitError != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	address, parseError := netip.ParseAddr(host)
	if parseError != nil {
		return host, netip.Addr{}, false
	}
	return host, address.Unmap(), true
}

func containsAddress(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
