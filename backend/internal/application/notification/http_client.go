package notification

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/netaddr"
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
		if !netaddr.IsPublic(address) {
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
