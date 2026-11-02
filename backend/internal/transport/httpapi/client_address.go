package httpapi

import (
	"net"
	"net/http"
	"net/netip"

	"github.com/vanquan3008/tracepilot/internal/config"
)

// clientAddress is the address that rate limits are keyed on; see
// config.ClientAddress for how forwarded headers are trusted.
func (server *Server) clientAddress(request *http.Request) string {
	return config.ClientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For"), server.trustedProxies)
}

// fromTrustedProxy reports whether the connection's peer is a configured reverse proxy, the only
// peer whose forwarded headers are believed.
func (server *Server) fromTrustedProxy(request *http.Request) bool {
	host, _, splitError := net.SplitHostPort(request.RemoteAddr)
	if splitError != nil {
		return false
	}
	address, parseError := netip.ParseAddr(host)
	if parseError != nil {
		return false
	}
	for _, prefix := range server.trustedProxies {
		if prefix.Contains(address.Unmap()) {
			return true
		}
	}
	return false
}
