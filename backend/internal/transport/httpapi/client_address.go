package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientAddress is the address that rate limits are keyed on. Forwarded headers
// are written by the client unless a proxy overwrites them, so X-Forwarded-For
// is read only when the TCP peer is a configured trusted proxy. It is read from
// the right: each hop appends the address it received from, so skipping the
// trusted proxies leaves the first address a trusted party actually saw. A
// leftmost value is whatever the client chose to send and is never used.
func (server *Server) clientAddress(request *http.Request) string {
	host, _, splitError := net.SplitHostPort(request.RemoteAddr)
	if splitError != nil {
		host = strings.TrimSpace(request.RemoteAddr)
	}
	peer, peerError := netip.ParseAddr(host)
	if peerError != nil {
		if host == "" {
			return "unknown"
		}
		return host
	}
	peer = peer.Unmap()
	if !server.isTrustedProxy(peer) {
		return peer.String()
	}
	hops := forwardedHops(request)
	for index := len(hops) - 1; index >= 0; index-- {
		hop, hopError := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if hopError != nil {
			return peer.String() // a malformed chain is not evidence of anything
		}
		if hop = hop.Unmap(); !server.isTrustedProxy(hop) {
			return hop.String()
		}
	}
	return peer.String()
}

func (server *Server) isTrustedProxy(address netip.Addr) bool {
	for _, prefix := range server.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// forwardedHops returns every X-Forwarded-For entry in order, across repeated headers.
func forwardedHops(request *http.Request) []string {
	var hops []string
	for _, header := range request.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(header, ",")...)
	}
	return hops
}
