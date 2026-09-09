package notification

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// safeClient returns the restricted HTTP client used for outbound webhooks.
// It never uses ambient proxy settings, follows redirects, or connects to a
// DNS answer outside the public address space.
func safeClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			ForceAttemptHTTP2:   false,
			TLSHandshakeTimeout: timeout,
			DialContext:         dialPublic,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, splitError := net.SplitHostPort(address)
	if splitError != nil {
		return nil, splitError
	}
	addresses, lookupError := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if lookupError != nil {
		return nil, lookupError
	}
	if len(addresses) == 0 {
		return nil, errors.New("webhook host resolved to no addresses")
	}
	for _, address := range addresses {
		if !isPublicAddr(address) {
			return nil, fmt.Errorf("webhook host resolves to a private or reserved address")
		}
	}
	var lastError error
	for _, address := range addresses {
		connection, dialError := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if dialError == nil {
			return connection, nil
		}
		lastError = dialError
	}
	return nil, lastError
}

func isPublicAddr(address netip.Addr) bool {
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	for _, prefix := range []string{
		"100.64.0.0/10",
		"169.254.0.0/16",
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"::ffff:0:0/96",
	} {
		if parsed, err := netip.ParsePrefix(prefix); err == nil && parsed.Contains(address) {
			return false
		}
	}
	return true
}
