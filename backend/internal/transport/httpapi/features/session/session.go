// Package session serves sign-in, token refresh, sign-out, the current account, and password changes.
package session

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	sessionapp "github.com/vanquan3008/tracepilot/internal/application/session"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	loginAttemptsPerEmail = 10
	// maxEmailLength is the longest address auth.NormalizeEmail accepts.
	maxEmailLength = 254
)

// Backend is what sessions read and change.
type Backend interface {
	sessionapp.CredentialReader
	sessionapp.PasswordStore
	api.MembershipLister
	CreateSession(ctx context.Context, userID, userAgent string) (string, auth.Session, error)
	RefreshSession(context.Context, string) (string, auth.User, error)
	RevokeSession(context.Context, string) error
	GetUser(context.Context, string) (auth.User, error)
	RecordLogin(context.Context, string) error
}

// Config signs access tokens and shapes the refresh cookie.
type Config struct {
	TokenSecret    []byte
	TokenIssuer    string
	TokenAudience  string
	AccessTokenTTL time.Duration

	RefreshTokenTTL time.Duration
	CookieDomain    string
	CookieSameSite  http.SameSite
	// The cookie is Secure in production, over TLS, or behind a trusted HTTPS proxy.
	Production     bool
	TLS            bool
	TrustedProxies []netip.Prefix
}

type Handler struct {
	backend Backend
	config  Config
	limiter *middleware.RequestRateLimiter
	logger  *slog.Logger
	errors  api.Errors
}

// New counts password attempts in limiter, the limiter the API's rate limits use.
func New(configuration Config, backend Backend, limiter *middleware.RequestRateLimiter, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, config: configuration, limiter: limiter, logger: logger, errors: api.Errors{Logger: logger}}
}

var refreshParameters = []string{"RefreshHeader", "RefreshCookie"}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/auth/refresh", Method: http.MethodPost, Scope: api.ScopePublic, Public: true, Tags: []string{"auth"}, OperationID: "refreshSession", SuccessStatus: http.StatusOK, Handler: handler.refresh, Headers: refreshParameters, Response: openapi.Object[sessionView]()},
		{Path: "/api/v1/auth/login", Method: http.MethodPost, Scope: api.ScopePublic, Public: true, Tags: []string{"auth"}, OperationID: "login", SuccessStatus: http.StatusOK, RequestBody: true, Handler: handler.login, Request: openapi.TypeOf[loginRequest](), Response: openapi.Object[sessionView]()},
		{Path: "/api/v1/auth/logout", Method: http.MethodPost, Scope: api.ScopePublic, Public: true, Tags: []string{"auth"}, OperationID: "logout", SuccessStatus: http.StatusNoContent, Handler: handler.logout, Headers: refreshParameters, Response: openapi.Empty()},
		{Path: "/api/v1/auth/me", Method: http.MethodGet, Scope: auth.ScopeSession, Tags: []string{"auth"}, OperationID: "getCurrentUser", Handler: handler.me, Response: openapi.Object[sessionView]()},
		{Path: "/api/v1/auth/password", Method: http.MethodPost, Scope: auth.ScopeSession, Tags: []string{"auth"}, OperationID: "changePassword", SuccessStatus: http.StatusOK, RequestBody: true, Handler: handler.changePassword, Request: openapi.TypeOf[changePasswordRequest](), Response: openapi.Object[sessionView]()},
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// sessionView is what the console needs after signing in or reloading. The access token is present
// when the response mints one.
type sessionView struct {
	User        auth.User         `json:"user"`
	Memberships []auth.Membership `json:"memberships"`
	AccessToken string            `json:"access_token,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

func (handler *Handler) login(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	var payload loginRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if len(email) > maxEmailLength {
		email = email[:maxEmailLength]
	}
	if !handler.allowAttempt(responseWriter, "login:"+email, "too many sign-in attempts; try again later") {
		return
	}
	user, loginError := sessionapp.Authenticate(request.Context(), handler.backend, email, payload.Password)
	if errors.Is(loginError, auth.ErrInvalidCredentials) {
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "INVALID_CREDENTIALS", auth.ErrInvalidCredentials.Error())
		return
	}
	if loginError != nil {
		handler.errors.Internal(responseWriter, request, "authenticate", loginError)
		return
	}
	refresh, _, sessionError := handler.backend.CreateSession(request.Context(), user.ID, request.UserAgent())
	if handler.errors.Identity(responseWriter, request, "create session", sessionError) {
		return
	}
	if recordError := handler.backend.RecordLogin(request.Context(), user.ID); recordError != nil {
		handler.logger.Warn("record login", "error", recordError)
	}
	handler.writeSession(responseWriter, request, user, refresh)
}

func (handler *Handler) refresh(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	next, user, refreshError := handler.backend.RefreshSession(request.Context(), refreshToken(request))
	if errors.Is(refreshError, auth.ErrSessionNotFound) {
		// Leave the cookie alone: when tabs refresh at once, the losing request carries the
		// token another request just rotated, and the browser may already hold its successor.
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "sign in again")
		return
	}
	if handler.errors.Identity(responseWriter, request, "refresh session", refreshError) {
		return
	}
	handler.writeSession(responseWriter, request, user, next)
}

func (handler *Handler) logout(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	if token := refreshToken(request); token != "" {
		if revokeError := handler.backend.RevokeSession(request.Context(), token); revokeError != nil {
			handler.errors.Internal(responseWriter, request, "revoke session", revokeError)
			return
		}
	}
	http.SetCookie(responseWriter, handler.refreshCookie(request, "", -1))
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) me(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	user, userError := handler.backend.GetUser(request.Context(), principal.UserID)
	if handler.errors.Identity(responseWriter, request, "get user", userError) {
		return
	}
	if view, ok := handler.sessionFor(responseWriter, request, user, false); ok {
		render.WriteJSON(responseWriter, http.StatusOK, view)
	}
}

func (handler *Handler) changePassword(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload changePasswordRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	if !handler.allowAttempt(responseWriter, "password:"+principal.UserID, "too many attempts; try again later") {
		return
	}
	changeError := sessionapp.ChangePassword(request.Context(), handler.backend, sessionapp.ChangePasswordCommand{
		UserID: principal.UserID, CurrentPassword: payload.CurrentPassword, NewPassword: payload.NewPassword,
		RefreshToken: refreshToken(request), Actor: principal.Actor(),
	})
	if errors.Is(changeError, auth.ErrInvalidCredentials) {
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "INVALID_CREDENTIALS", "current password is incorrect")
		return
	}
	if handler.errors.Identity(responseWriter, request, "change password", changeError) {
		return
	}
	user, userError := handler.backend.GetUser(request.Context(), principal.UserID)
	if handler.errors.Identity(responseWriter, request, "get user", userError) {
		return
	}
	if view, ok := handler.sessionFor(responseWriter, request, user, true); ok {
		render.WriteJSON(responseWriter, http.StatusOK, view)
	}
}

// writeSession answers a sign-in or refresh with a new access token and the rotated refresh cookie.
func (handler *Handler) writeSession(responseWriter http.ResponseWriter, request *http.Request, user auth.User, refresh string) {
	view, ok := handler.sessionFor(responseWriter, request, user, true)
	if !ok {
		return
	}
	handler.setRefreshCookie(responseWriter, request, refresh)
	render.WriteJSON(responseWriter, http.StatusOK, view)
}

// sessionFor builds what the console shows for a signed-in account. With issueToken it also mints an
// access token; otherwise the response leaves the token fields out.
func (handler *Handler) sessionFor(responseWriter http.ResponseWriter, request *http.Request, user auth.User, issueToken bool) (sessionView, bool) {
	memberships, listError := api.VisibleProjects(request.Context(), handler.backend, user)
	if handler.errors.Identity(responseWriter, request, "list projects", listError) {
		return sessionView{}, false
	}
	view := sessionView{User: user, Memberships: memberships}
	if !issueToken {
		return view, true
	}
	token, expiresAt, tokenError := handler.issueAccessToken(user)
	if tokenError != nil {
		handler.errors.Internal(responseWriter, request, "sign access token", tokenError)
		return sessionView{}, false
	}
	view.AccessToken, view.ExpiresAt = token, &expiresAt
	return view, true
}

func (handler *Handler) issueAccessToken(user auth.User) (string, time.Time, error) {
	ttl := handler.config.AccessTokenTTL
	if ttl <= 0 {
		ttl = auth.AccessTokenTTL
	}
	return auth.IssueAccessTokenWithOptions(handler.config.TokenSecret, user.ID, time.Now(), ttl, handler.config.TokenIssuer, handler.config.TokenAudience, user.TokenVersion())
}

// allowAttempt counts one password attempt against key and answers 429 once the key is out of attempts.
func (handler *Handler) allowAttempt(responseWriter http.ResponseWriter, key, message string) bool {
	allowed, retryAfter := handler.limiter.Allow(key, loginAttemptsPerEmail, time.Now())
	if allowed {
		return true
	}
	responseWriter.Header().Set("Retry-After", middleware.RetryAfterSeconds(retryAfter))
	render.WriteAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", message)
	return false
}
