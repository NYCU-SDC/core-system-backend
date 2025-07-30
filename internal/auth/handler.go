package auth

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth/oauthprovider"
	"NYCU-SDC/core-system-backend/internal/jwt"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

const (
	AccessTokenCookieName  = "access_token"
	RefreshTokenCookieName = "refresh_token"
)

type JWTIssuer interface {
	New(ctx context.Context, user user.User) (string, error)
	Parse(ctx context.Context, tokenString string) (user.User, error)
	GenerateRefreshToken(ctx context.Context, userID uuid.UUID) (jwt.RefreshToken, error)
	GetUserIDByRefreshToken(ctx context.Context, refreshTokenID uuid.UUID) (uuid.UUID, error)
}

type JWTStore interface {
	InactivateRefreshToken(ctx context.Context, id uuid.UUID) error
	GetRefreshTokenByID(ctx context.Context, id uuid.UUID) (jwt.RefreshToken, error)
}

type UserStore interface {
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	GetByID(ctx context.Context, id uuid.UUID) (user.User, error)
	FindOrCreate(ctx context.Context, name, username, avatarUrl string, role []string, oauthProvider, oauthProviderID string) (uuid.UUID, error)
}

type OAuthProvider interface {
	Name() string
	Config() *oauth2.Config
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, error)
}

type callBackInfo struct {
	code       string
	oauthError string
	callback   url.URL
	redirectTo string
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	userStore UserStore
	jwtIssuer JWTIssuer
	jwtStore  JWTStore
	provider  map[string]OAuthProvider

	accessTokenExpiration  time.Duration
	refreshTokenExpiration time.Duration
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	userStore UserStore,
	jwtIssuer JWTIssuer,
	jwtStore JWTStore,

	oauthProxyBaseURL string,
	accessTokenExpiration time.Duration,
	refreshTokenExpiration time.Duration,
	googleOauthConfig oauthprovider.GoogleOauth,
) *Handler {

	return &Handler{
		logger: logger,
		tracer: otel.Tracer("auth/handler"),

		validator:     validator,
		problemWriter: problemWriter,

		userStore: userStore,
		jwtIssuer: jwtIssuer,
		jwtStore:  jwtStore,
		provider: map[string]OAuthProvider{
			"google": oauthprovider.NewGoogleConfig(
				googleOauthConfig.ClientID,
				googleOauthConfig.ClientSecret,
				fmt.Sprintf("%s/auth/google/callback", oauthProxyBaseURL),
			),
		},

		accessTokenExpiration:  accessTokenExpiration,
		refreshTokenExpiration: refreshTokenExpiration,
	}
}

// Oauth2Start initiates the OAuth2 flow by redirecting the user to the provider's authorization URL
func (h *Handler) Oauth2Start(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Oauth2Start")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	providerName := r.PathValue("provider")
	provider := h.provider[providerName]
	if provider == nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: provider not found: %s", internal.ErrProviderNotFound, providerName), logger)
		return
	}

	// The final destination for the user, after authentication is complete.
	callbackURL := r.URL.Query().Get("c")
	finalRedirectURL := r.URL.Query().Get("r")

	// The URL for the oauth-proxy to call back to. This is our own backend's callback endpoint.
	// We construct it dynamically based on the current request.
	if callbackURL == ""{
		// set the callback URL to the current URL with the path /callback
		callbackURL = r.URL.String() + "/callback"
	}
	if finalRedirectURL != ""{
		callbackURL = fmt.Sprintf("%s?r=%s", callbackURL, finalRedirectURL)
	}
	// The state payload contains the backend callback URL and the final redirect URL.
	// This will be consumed by the oauth-proxy.
	state := base64.StdEncoding.EncodeToString([]byte(callbackURL))
	
	authURL := provider.Config().AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)

	logger.Info("Redirecting to Google OAuth2", zap.String("url", authURL))
}

// Callback handles the OAuth2 callback from oauth-proxy
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {

	traceCtx, span := h.tracer.Start(r.Context(), "Callback")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	providerName := r.PathValue("provider")
	provider := h.provider[providerName]
	if provider == nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: provider not found: %s", internal.ErrProviderNotFound, providerName), logger)
		return
	}

	// Get the OAuth2 code and state from the request
	callbackInfo, err := h.getCallBackInfo(r.URL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidCallbackInfo, err), logger)
		return
	}

	h.logger.Info("Get Callback from oauth-proxy", zap.Any("callback", callbackInfo.callback))

	callback := callbackInfo.callback.String()
	code := callbackInfo.code
	redirectTo := callbackInfo.redirectTo
	oauthError := callbackInfo.oauthError

	if oauthError != "" {
		http.Redirect(w, r, fmt.Sprintf("%s?error=%s", callback, oauthError), http.StatusFound)
		return
	}

	token, err := provider.Exchange(traceCtx, code)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidExchangeToken, err), logger)
		return
	}

	userInfo, auth, err := provider.GetUserInfo(traceCtx, token)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	userID, err := h.userStore.FindOrCreate(traceCtx, userInfo.Name.String, userInfo.Username.String, userInfo.AvatarUrl.String, userInfo.Role, providerName, auth.ProviderID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	accessTokenID, refreshTokenID, err := h.generateJWT(traceCtx, userID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, accessTokenID, refreshTokenID)

	redirectURL := redirectTo
	if redirectURL == "" {
		redirectURL = "/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) DebugToken(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DebugToken")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	e := r.URL.Query().Get("error")
	if e != "" {
		h.problemWriter.WriteError(traceCtx, w, handlerutil.ErrForbidden, logger)
		return
	}

	accessTokenCookie, err := r.Cookie(AccessTokenCookieName)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, errors.New("missing access token cookie"), logger)
		return
	}

	token := accessTokenCookie.Value
	if token == "" {
		h.problemWriter.WriteError(traceCtx, w, errors.New("empty access token cookie"), logger)
		return
	}

	jwtUser, err := h.jwtIssuer.Parse(traceCtx, token)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, jwtUser)
}

func (h *Handler) generateJWT(ctx context.Context, userID uuid.UUID) (string, string, error) {
	traceCtx, span := h.tracer.Start(ctx, "generateJWT")
	defer span.End()

	userEntity, err := h.userStore.GetByID(traceCtx, userID)
	if err != nil {
		return "", "", err
	}

	jwtToken, err := h.jwtIssuer.New(traceCtx, userEntity)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := h.jwtIssuer.GenerateRefreshToken(traceCtx, userID)
	if err != nil {
		return "", "", err
	}

	return jwtToken, refreshToken.ID.String(), nil
}

func (h *Handler) getCallBackInfo(url *url.URL) (callBackInfo, error) {

	code := url.Query().Get("code")
	state := url.Query().Get("state")
	oauthError := url.Query().Get("error")

	callbackURL, err := base64.StdEncoding.DecodeString(state)
	if err != nil {
		return callBackInfo{}, err
	}

	callback, err := url.Parse(string(callbackURL))
	if err != nil {
		return callBackInfo{}, err
	}

	// Clear the query parameters from the callback URL, due to "?" symbol in original URL
	redirectTo := callback.Query().Get("r")
	callback.RawQuery = ""

	return callBackInfo{
		code:       code,
		oauthError: oauthError,
		callback:   *callback,
		redirectTo: redirectTo,
	}, nil
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Logout")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Inactivate the current refresh token from cookie
	refreshTokenCookie, err := r.Cookie(RefreshTokenCookieName)
	if err == nil && refreshTokenCookie.Value != "" {
		refreshTokenID, err := uuid.Parse(refreshTokenCookie.Value)
		if err == nil {
			err = h.jwtStore.InactivateRefreshToken(traceCtx, refreshTokenID)
			if err != nil {
				logger.Warn("Failed to inactivate refresh token during logout", zap.Error(err))
			}
		}
	}

	h.clearAccessAndRefreshCookies(w)

	handlerutil.WriteJSONResponse(w, http.StatusOK, map[string]string{"message": "Successfully logged out"})
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RefreshToken")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Read refresh token from cookie instead of path parameter
	refreshTokenCookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, logger)
		return
	}
	refreshTokenStr := refreshTokenCookie.Value

	if refreshTokenStr == "" {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, logger)
		return
	}

	refreshTokenID, err := uuid.Parse(refreshTokenStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, logger)
		return
	}

	userID, err := h.jwtIssuer.GetUserIDByRefreshToken(traceCtx, refreshTokenID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidRefreshToken, logger)
		return
	}

	err = h.jwtStore.InactivateRefreshToken(traceCtx, refreshTokenID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	newAccessTokenID, newRefreshTokenID, err := h.generateJWT(traceCtx, userID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, newAccessTokenID, newRefreshTokenID)

	w.WriteHeader(http.StatusNoContent)
}

// InternalAPITokenLogin handles login using an internal API token, Todo: this handler need to be protected by an API token or feature flag
func (h *Handler) InternalAPITokenLogin(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "APITokenLogin")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse and validate the request body
	var req struct {
		UserIDStr string `json:"uid" validate:"required"`
	}
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	uid, err := uuid.Parse(req.UserIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, logger)
		return
	}

	exists, err := h.userStore.ExistsByID(traceCtx, uid)
	if err != nil || !exists {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrUserNotFound, logger)
		return
	}

	jwtToken, refreshTokenID, err := h.generateJWT(traceCtx, uid)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidJWTToken, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, jwtToken, refreshTokenID)

	handlerutil.WriteJSONResponse(w, http.StatusOK, map[string]string{"message": "Login successful"})
}

// setAccessAndRefreshCookies sets the access/refresh cookies with HTTP-only and secure flags
func (h *Handler) setAccessAndRefreshCookies(w http.ResponseWriter, accessTokenID, refreshTokenID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessTokenID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   int(h.accessTokenExpiration.Seconds()),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshTokenID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/api/auth/refresh",
		MaxAge:   int(h.refreshTokenExpiration.Seconds()),
	})
}

// clearAccessAndRefreshCookies sets the access/refresh cookies to empty values and negative MaxAge
// negative means the cookies will be deleted, zero means the cookies will expire at the end of the session
func (h *Handler) clearAccessAndRefreshCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     "/api/auth/refresh",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}
