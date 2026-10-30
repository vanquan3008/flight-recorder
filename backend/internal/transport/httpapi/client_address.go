package httpapi

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/config"
)

// clientAddress is the address that rate limits are keyed on; see
// config.ClientAddress for how forwarded headers are trusted.
func (server *Server) clientAddress(request *http.Request) string {
	return config.ClientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For"), server.trustedProxies)
}
