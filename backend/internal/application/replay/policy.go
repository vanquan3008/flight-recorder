package replay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/netaddr"
)

type Policy struct {
	allowedHosts map[string]struct{}
	allowPrivate bool
	lookup       func(context.Context, string) ([]net.IPAddr, error)
}

func NewPolicy(allowedHosts []string, allowPrivate bool) (*Policy, error) {
	hosts := make(map[string]struct{}, len(allowedHosts))
	for _, host := range allowedHosts {
		normalized := strings.ToLower(strings.TrimSpace(host))
		if normalized == "" || strings.Contains(normalized, "://") || strings.Contains(normalized, "/") {
			return nil, fmt.Errorf("invalid replay allowed host %q", host)
		}
		hosts[normalized] = struct{}{}
	}
	if len(hosts) == 0 {
		return nil, errors.New("replay requires at least one allowed host")
	}
	return &Policy{allowedHosts: hosts, allowPrivate: allowPrivate, lookup: net.DefaultResolver.LookupIPAddr}, nil
}

func (policy *Policy) Validate(ctx context.Context, rawTarget string) (*url.URL, error) {
	target, parseError := url.Parse(rawTarget)
	if parseError != nil || target.Scheme == "" || target.Host == "" {
		return nil, errors.New("replay target must be an absolute URL")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("replay target must use HTTP or HTTPS")
	}
	if target.User != nil || target.Fragment != "" {
		return nil, errors.New("replay target contains forbidden URL components")
	}
	if validateError := policy.validateHost(ctx, target.Hostname()); validateError != nil {
		return nil, validateError
	}
	return target, nil
}

func (policy *Policy) HTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, splitError := net.SplitHostPort(address)
		if splitError != nil {
			return nil, splitError
		}
		addresses, resolveError := policy.addresses(ctx, host)
		if resolveError != nil {
			return nil, resolveError
		}
		for _, resolved := range addresses {
			connection, dialError := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
			if dialError == nil {
				return connection, nil
			}
		}
		return nil, errors.New("replay target cannot be reached")
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (policy *Policy) validateHost(ctx context.Context, host string) error {
	_, resolveError := policy.addresses(ctx, host)
	return resolveError
}

func (policy *Policy) addresses(ctx context.Context, host string) ([]netip.Addr, error) {
	normalized := strings.ToLower(strings.TrimSpace(host))
	if _, allowed := policy.allowedHosts[normalized]; !allowed {
		return nil, errors.New("replay target host is not allowed")
	}
	if address, parseError := netip.ParseAddr(normalized); parseError == nil {
		if policy.allowPrivate || netaddr.IsPublic(address) {
			return []netip.Addr{address}, nil
		}
		return nil, errors.New("replay target resolves to a private address")
	}
	addresses, lookupError := policy.lookup(ctx, normalized)
	if lookupError != nil || len(addresses) == 0 {
		return nil, errors.New("replay target host cannot be resolved")
	}
	resolved := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		parsed, valid := netip.AddrFromSlice(address.IP)
		if !valid || (!policy.allowPrivate && !netaddr.IsPublic(parsed)) {
			return nil, errors.New("replay target resolves to a private address")
		}
		resolved = append(resolved, parsed)
	}
	return resolved, nil
}
