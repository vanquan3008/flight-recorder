package session

import (
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

const (
	refreshCookieName = "tp_refresh"
	refreshHeader     = "X-TracePilot-Refresh"
	refreshPath       = "/api/v1/auth"
)

// refreshToken reads the refresh cookie only when the console also sends the refresh header, so a
// cross-site form post cannot use the cookie.
func refreshToken(request *http.Request) string {
	if request.Header.Get(refreshHeader) == "" {
		return ""
	}
	if cookie, cookieError := request.Cookie(refreshCookieName); cookieError == nil {
		return cookie.Value
	}
	return ""
}

func (handler *Handler) setRefreshCookie(responseWriter http.ResponseWriter, request *http.Request, token string) {
	maxAge := int(handler.config.RefreshTokenTTL.Seconds())
	if maxAge <= 0 {
		maxAge = int((30 * 24 * time.Hour).Seconds())
	}
	http.SetCookie(responseWriter, handler.refreshCookie(request, token, maxAge))
}

// refreshCookie scopes the refresh token to the auth routes and keeps it away from scripts.
func (handler *Handler) refreshCookie(request *http.Request, value string, maxAge int) *http.Cookie {
	sameSite := handler.config.CookieSameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}
	return &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshPath,
		Domain:   handler.config.CookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   handler.cookieSecure(request),
		SameSite: sameSite,
	}
}

func (handler *Handler) cookieSecure(request *http.Request) bool {
	if handler.config.CookieSameSite == http.SameSiteNoneMode {
		return true
	}
	if handler.config.Production || request.TLS != nil || handler.config.TLS {
		return true
	}
	return strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https") && config.FromTrustedProxy(request.RemoteAddr, handler.config.TrustedProxies)
}
