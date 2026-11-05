package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

const (
	// refreshCookieName holds the opaque refresh token. It is only ever sent to the auth routes.
	refreshCookieName = "tp_refresh"
	// refreshHeader must accompany a request that uses the refresh cookie. A page on another site
	// cannot add a custom header to a cross-site request without a CORS preflight this server refuses,
	// so the cookie alone cannot be used to act on a signed-in person's behalf.
	refreshHeader = "X-TracePilot-Refresh"
	refreshPath   = "/api/v1/auth"
)

var authRoutes = []apiRouteDefinition{
	{path: "/api/v1/auth/status", method: http.MethodGet, scope: "public", public: true, tags: []string{"auth"}, operationID: "getAuthStatus", handler: (*Server).authStatus},
	{path: "/api/v1/auth/register", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "register", successStatus: http.StatusCreated, requestBody: true, handler: (*Server).authRegister},
	{path: "/api/v1/auth/refresh", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "refreshSession", successStatus: http.StatusOK, handler: (*Server).authRefresh},
	{path: "/api/v1/auth/login", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "login", successStatus: http.StatusOK, requestBody: true, handler: (*Server).authLogin},
	{path: "/api/v1/auth/logout", method: http.MethodPost, scope: "public", public: true, tags: []string{"auth"}, operationID: "logout", successStatus: http.StatusNoContent, handler: (*Server).authLogout},
	{path: "/api/v1/auth/me", method: http.MethodGet, scope: auth.ScopeSession, tags: []string{"auth"}, operationID: "getCurrentUser", handler: (*Server).authMe},
	{path: "/api/v1/auth/password", method: http.MethodPost, scope: auth.ScopeSession, tags: []string{"auth"}, operationID: "changePassword", successStatus: http.StatusOK, requestBody: true, handler: (*Server).authChangePassword},
}

func (server *Server) authStatus(responseWriter http.ResponseWriter, _ *http.Request, _ auth.Principal) {
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"registration_open": server.allowRegistration, "first_account": false})
}

// signInView finishes a sign-in: it opens a refresh session, sets its cookie and answers with the
// access token and the projects the person can open.
func (server *Server) signInView(responseWriter http.ResponseWriter, request *http.Request, user auth.User, status int) {
	refresh, _, sessionError := server.backend.CreateSession(request.Context(), user.ID, request.UserAgent())
	if server.writeIdentityError(responseWriter, "create session", sessionError) {
		return
	}
	if recordError := server.backend.RecordLogin(request.Context(), user.ID); recordError != nil {
		server.logger.Warn("record login", "error", recordError)
	}
	server.writeSession(responseWriter, request, user, refresh, status)
}

// writeSession sets the refresh cookie and answers with a fresh access token.
func (server *Server) writeSession(responseWriter http.ResponseWriter, request *http.Request, user auth.User, refresh string, status int) {
	memberships, listError := server.projectsFor(request, user)
	if server.writeIdentityError(responseWriter, "list projects", listError) {
		return
	}
	ttl := server.accessTokenTTL
	if ttl <= 0 {
		ttl = auth.AccessTokenTTL
	}
	token, expires, tokenError := auth.IssueAccessTokenWithOptions(server.jwtSecret, user.ID, time.Now(), ttl, server.issuer, server.audience)
	if tokenError != nil {
		server.writeInternalError(responseWriter, "sign access token", tokenError)
		return
	}
	server.setRefreshCookie(responseWriter, request, refresh)
	writeJSONValue(responseWriter, status, sessionView{User: user, Memberships: memberships, AccessToken: token, ExpiresAt: &expires})
}

// refreshToken returns the caller's refresh cookie, or "" when the request does not prove it came
// from the console (see refreshHeader).
func refreshToken(request *http.Request) string {
	if request.Header.Get(refreshHeader) == "" {
		return ""
	}
	if cookie, cookieError := request.Cookie(refreshCookieName); cookieError == nil {
		return cookie.Value
	}
	return ""
}

func (server *Server) cookieSecure(request *http.Request) bool {
	if server.cookieSameSite == http.SameSiteNoneMode {
		return true
	}
	if server.production || request.TLS != nil || server.certificateFile != "" {
		return true
	}
	return strings.EqualFold(request.Header.Get("X-Forwarded-Proto"), "https") && config.FromTrustedProxy(request.RemoteAddr, server.trustedProxies)
}

func (server *Server) setRefreshCookie(responseWriter http.ResponseWriter, request *http.Request, token string) {
	maxAge := int(server.refreshTokenTTL.Seconds())
	if maxAge <= 0 {
		maxAge = int((30 * 24 * time.Hour).Seconds())
	}
	sameSite := server.cookieSameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshPath,
		Domain:   server.cookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   server.cookieSecure(request),
		SameSite: sameSite,
	})
}

func (server *Server) clearRefreshCookie(responseWriter http.ResponseWriter, request *http.Request) {
	sameSite := server.cookieSameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}
	http.SetCookie(responseWriter, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshPath,
		Domain:   server.cookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   server.cookieSecure(request),
		SameSite: sameSite,
	})
}

// authRegister creates an ordinary account when registration is open and signs the user in.
// Platform administrators can promote ordinary accounts or grant them project access.
func (server *Server) authRegister(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	var payload struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	// Registration is open to anyone who can reach the server, so it is throttled per address by the router.
	user, registerError := server.backend.RegisterUser(request.Context(), server.defaultProjectID, auth.CreateUserParams{
		Email: payload.Email, DisplayName: payload.DisplayName, Password: payload.Password,
	}, server.allowRegistration)
	if server.writeIdentityError(responseWriter, "register", registerError) {
		return
	}
	server.signInView(responseWriter, request, user, http.StatusCreated)
}

func (server *Server) authLogin(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	var payload struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	// The address becomes a rate-limit key, and no valid one is longer than this; cutting it keeps a
	// request body from filling the limiter with large keys.
	if len(email) > maxEmailLength {
		email = email[:maxEmailLength]
	}
	// Guessing is limited per client by the router and per account here, so spreading attempts
	// across addresses does not help.
	if allowed, retryAfter := server.rateLimiter.Allow("login:"+email, loginAttemptsPerEmail, time.Now()); !allowed {
		responseWriter.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
		writeAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", "too many sign-in attempts; try again later")
		return
	}
	credentials, lookupError := server.backend.GetCredentials(request.Context(), email)
	hash := auth.DummyPasswordHash()
	if lookupError == nil {
		hash = credentials.PasswordHash
	} else if !errors.Is(lookupError, auth.ErrUserNotFound) {
		server.writeInternalError(responseWriter, "load credentials", lookupError)
		return
	}
	// Verify even for an unknown email so the response time does not reveal which accounts exist.
	valid := len(payload.Password) <= auth.MaxPasswordLength*4 && auth.VerifyPassword(payload.Password, hash)
	if lookupError != nil || !valid || credentials.User.DisabledAt != nil {
		writeAPIError(responseWriter, http.StatusUnauthorized, "INVALID_CREDENTIALS", auth.ErrInvalidCredentials.Error())
		return
	}
	server.signInView(responseWriter, request, credentials.User, http.StatusOK)
}

// authRefresh trades the refresh cookie for a new one and a new access token. The console calls it when
// its access token expires, and on page load to resume a session.
func (server *Server) authRefresh(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	next, user, refreshError := server.backend.RefreshSession(request.Context(), refreshToken(request))
	if errors.Is(refreshError, auth.ErrSessionNotFound) {
		server.clearRefreshCookie(responseWriter, request)
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "sign in again")
		return
	}
	if server.writeIdentityError(responseWriter, "refresh session", refreshError) {
		return
	}
	server.writeSession(responseWriter, request, user, next, http.StatusOK)
}

func (server *Server) authLogout(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	if token := refreshToken(request); token != "" {
		if revokeError := server.backend.RevokeSession(request.Context(), token); revokeError != nil {
			server.writeInternalError(responseWriter, "revoke session", revokeError)
			return
		}
	}
	server.clearRefreshCookie(responseWriter, request)
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (server *Server) authMe(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	user, userError := server.backend.GetUser(request.Context(), principal.UserID)
	if server.writeIdentityError(responseWriter, "get user", userError) {
		return
	}
	memberships, listError := server.projectsFor(request, user)
	if server.writeIdentityError(responseWriter, "list projects", listError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, sessionView{User: user, Memberships: memberships})
}

// authChangePassword replaces the password, ends every other session and the access tokens issued so
// far, and answers with a new access token so this one stays signed in.
func (server *Server) authChangePassword(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	if allowed, retryAfter := server.rateLimiter.Allow("password:"+principal.UserID, loginAttemptsPerEmail, time.Now()); !allowed {
		responseWriter.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
		writeAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts; try again later")
		return
	}
	credentials, loadError := server.backend.GetCredentialsByID(request.Context(), principal.UserID)
	if server.writeIdentityError(responseWriter, "load credentials", loadError) {
		return
	}
	if len(payload.CurrentPassword) > auth.MaxPasswordLength*4 || !auth.VerifyPassword(payload.CurrentPassword, credentials.PasswordHash) {
		writeAPIError(responseWriter, http.StatusUnauthorized, "INVALID_CREDENTIALS", "current password is incorrect")
		return
	}
	if payload.NewPassword == payload.CurrentPassword {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "choose a password different from the current one")
		return
	}
	// The refresh cookie, when the console sends it, keeps this browser's session alive.
	keep := refreshToken(request)
	if setError := server.backend.SetPassword(request.Context(), principal.UserID, payload.NewPassword, false, keep, principal.Actor()); server.writeIdentityError(responseWriter, "set password", setError) {
		return
	}
	if server.identity != nil {
		server.identity.forgetUser(principal.UserID)
	}
	user, userError := server.backend.GetUser(request.Context(), principal.UserID)
	if server.writeIdentityError(responseWriter, "get user", userError) {
		return
	}
	memberships, listError := server.projectsFor(request, user)
	if server.writeIdentityError(responseWriter, "list projects", listError) {
		return
	}
	token, expires, tokenError := auth.IssueAccessToken(server.jwtSecret, user.ID, time.Now())
	if tokenError != nil {
		server.writeInternalError(responseWriter, "sign access token", tokenError)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, sessionView{User: user, Memberships: memberships, AccessToken: token, ExpiresAt: &expires})
}
