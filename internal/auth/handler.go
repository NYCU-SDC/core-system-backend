package auth

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth/oauthprovider"
	"NYCU-SDC/core-system-backend/internal/jwt"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	LinkTokenCookieName    = "link_token"
)

type JWTIssuer interface {
	New(ctx context.Context, user user.User) (string, error)
	NewState(ctx context.Context, service, environment, callbackURL, redirectURL string) (string, error)
	NewFormState(ctx context.Context, callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string) (string, error)
	NewLinkToken(ctx context.Context, provider, providerID, userID string) (string, error)
	Parse(ctx context.Context, tokenString string) (user.User, error)
	ParseState(ctx context.Context, tokenString string) (*jwt.OauthProxyClaims, error)
	ParseFormState(ctx context.Context, tokenString string) (callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string, err error)
	ParseLinkToken(ctx context.Context, tokenString string) (provider, providerID, userID string, err error)
	GenerateRefreshToken(ctx context.Context, userID uuid.UUID) (jwt.RefreshToken, error)
	GetUserIDByRefreshToken(ctx context.Context, refreshTokenID uuid.UUID) (uuid.UUID, error)
}

type JWTStore interface {
	InactivateRefreshToken(ctx context.Context, id uuid.UUID) error
	GetRefreshTokenByID(ctx context.Context, id uuid.UUID) (jwt.RefreshToken, error)
}

type UserStore interface {
	ExistsByID(ctx context.Context, id uuid.UUID) (bool, error)
	GetByID(ctx context.Context, id uuid.UUID) (user.UsersWithEmail, error)
	FindOrCreate(ctx context.Context, name, username, avatarUrl string, email string, role []string, oauthProvider, oauthProviderID string) (user.FindOrCreateResult, error)
	CreateAuth(ctx context.Context, userID uuid.UUID, provider, providerID string) error
	CreateEmail(ctx context.Context, userID uuid.UUID, email string) error
}

type OAuthProvider interface {
	Name() string
	Config() *oauth2.Config
	ConfigWithCustomRedirectURL(redirectURL string) *oauth2.Config
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, string, error)
}

type callBackInfo struct {
	code        string
	oauthError  string
	proxyClaims *jwt.OauthProxyClaims
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	baseURL           string
	oauthProxyBaseURL string
	environment       string
	devMode           bool

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

	baseURL string,
	oauthProxyBaseURL string,
	environment string,
	devMode bool,

	accessTokenExpiration time.Duration,
	refreshTokenExpiration time.Duration,
	googleOauthConfig oauthprovider.GoogleOauth,
	nycuOauthConfig oauthprovider.NYCUOauth,
) *Handler {
	getCallbackURL := func(provider string) string {
		if oauthProxyBaseURL != "" {
			return fmt.Sprintf("%s/api/auth/%s/callback", oauthProxyBaseURL, provider)
		}
		return fmt.Sprintf("%s/api/auth/login/oauth/%s/callback", baseURL, provider)
	}
	googleOauthCallbackURL := getCallbackURL("google")
	nycuOauthCallbackURL := getCallbackURL("nycu")

	return &Handler{
		logger: logger,
		tracer: otel.Tracer("auth/handler"),

		baseURL:           baseURL,
		oauthProxyBaseURL: oauthProxyBaseURL,
		environment:       environment,
		devMode:           devMode,

		validator:     validator,
		problemWriter: problemWriter,

		userStore: userStore,
		jwtIssuer: jwtIssuer,
		jwtStore:  jwtStore,
		provider: map[string]OAuthProvider{
			"google": oauthprovider.NewGoogleConfig(
				googleOauthConfig.ClientID,
				googleOauthConfig.ClientSecret,
				googleOauthCallbackURL,
			),
			"nycu": oauthprovider.NewNYCUConfig(
				nycuOauthConfig.ClientID,
				nycuOauthConfig.ClientSecret,
				nycuOauthCallbackURL,
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

	// Redirect URL after successful login
	redirectURL := r.URL.Query().Get("r")

	// Determine callback URL based on oauth proxy configuration
	callbackURL := ""
	if h.oauthProxyBaseURL != "" {
		baseForCallback := h.baseURL
		if h.devMode {
			customBase := r.URL.Query().Get("base")
			if customBase != "" {
				baseForCallback = strings.TrimRight(customBase, "/")
			}
		}
		callbackURL = fmt.Sprintf("%s/api/auth/login/oauth/%s/callback", baseForCallback, providerName)
		logger.Info(callbackURL)
	} else {
		if h.devMode {
			customBase := r.URL.Query().Get("base")
			if customBase != "" {
				callbackURL = fmt.Sprintf("%s/api/auth/login/oauth/%s/callback", strings.TrimRight(customBase, "/"), providerName)
			}
		}
	}

	// Create JWT state for OAuth flow
	state, err := h.jwtIssuer.NewState(traceCtx, "core-system", h.environment, callbackURL, redirectURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrNewStateFailed, err), logger)
		return
	}

	// Generate OAuth authorization URL and redirect
	var config *oauth2.Config
	if h.oauthProxyBaseURL == "" && callbackURL != "" {
		config = provider.ConfigWithCustomRedirectURL(callbackURL)
	} else {
		config = provider.Config()
	}

	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

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
	callbackInfo, err := h.getCallBackInfo(traceCtx, r.URL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidCallbackInfo, err), logger)
		return
	}

	code := callbackInfo.code
	redirectTo := callbackInfo.proxyClaims.RedirectURL
	callbackURL := callbackInfo.proxyClaims.CallbackURL
	oauthError := callbackInfo.oauthError

	if oauthError != "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %s", internal.ErrOAuthError, oauthError), logger)
		return
	}

	var token *oauth2.Token
	if callbackURL != "" {
		config := provider.ConfigWithCustomRedirectURL(callbackURL)
		token, err = config.Exchange(traceCtx, code)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidExchangeToken, err), logger)
			return
		}
	} else {
		token, err = provider.Exchange(traceCtx, code)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidExchangeToken, err), logger)
			return
		}
	}

	userInfo, auth, email, err := provider.GetUserInfo(traceCtx, token)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	result, err := h.userStore.FindOrCreate(traceCtx, userInfo.Name.String, userInfo.Username.String, userInfo.AvatarUrl.String, email, userInfo.Role, providerName, auth.ProviderID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	// Email conflict detected: a different provider already has this email.
	// Do not issue access tokens yet; set a linking cookie and
	// redirect to the binding confirmation page.
	if result.ExistingProvider != "" {
		linkToken, err := h.jwtIssuer.NewLinkToken(traceCtx, auth.Provider, auth.ProviderID, result.UserID.String())
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
			return
		}
		h.setLinkCookie(w, baseURL.Host, linkToken)

		redirectURL := redirectTo
		if redirectURL == "" {
			redirectURL = fmt.Sprintf("/link?name=%s&oauthProvider=%s&email=%s", result.ExistingName, result.ExistingProvider, email)
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Normal login: issue access and refresh tokens.
	if email != "" {
		err := h.userStore.CreateEmail(traceCtx, result.UserID, email)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, internal.ErrFailedToCreateEmail, logger)
			return
		}
	}

	accessTokenID, refreshTokenID, err := h.generateJWT(traceCtx, result.UserID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, baseURL.Host, accessTokenID, refreshTokenID)

	redirectURL := redirectTo
	if redirectURL == "" {
		if h.environment == "snapshot" || h.environment == "no-env" {
			redirectURL = "/api/users/me"
		} else {
			redirectURL = "/"
		}
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) generateJWT(ctx context.Context, userID uuid.UUID) (string, string, error) {
	traceCtx, span := h.tracer.Start(ctx, "generateJWT")
	defer span.End()

	userEntityRow, err := h.userStore.GetByID(traceCtx, userID)
	if err != nil {
		return "", "", err
	}

	// Convert GetByIDRow to user.User expected by JWTIssuer
	userEntity := user.User{
		ID:        userEntityRow.ID,
		Name:      userEntityRow.Name,
		Username:  userEntityRow.Username,
		AvatarUrl: userEntityRow.AvatarUrl,
		Role:      userEntityRow.Role,
		CreatedAt: userEntityRow.CreatedAt,
		UpdatedAt: userEntityRow.UpdatedAt,
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

func (h *Handler) getCallBackInfo(ctx context.Context, url *url.URL) (callBackInfo, error) {
	code := url.Query().Get("code")
	state := url.Query().Get("state")
	oauthError := url.Query().Get("error")

	oauthProxyClaims, err := h.jwtIssuer.ParseState(ctx, state)
	if err != nil {
		return callBackInfo{}, err
	}

	return callBackInfo{
		code:        code,
		oauthError:  oauthError,
		proxyClaims: oauthProxyClaims,
	}, nil
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Logout")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}
	domain := baseURL.Host

	// Inactivate the current refresh token from cookie
	refreshTokenCookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil {
		logger.Error("Failed to get refresh token cookie during logout", zap.Error(err))
		h.clearAccessAndRefreshCookies(w, domain)
		return
	}

	refreshTokenID, err := uuid.Parse(refreshTokenCookie.Value)
	if err != nil {
		logger.Error("Invalid refresh token format during logout", zap.Error(err))
		h.clearAccessAndRefreshCookies(w, domain)
		return
	}

	err = h.jwtStore.InactivateRefreshToken(traceCtx, refreshTokenID)
	if err != nil {
		logger.Warn("Failed to inactivate refresh token during logout", zap.Error(err))
		h.clearAccessAndRefreshCookies(w, domain)
		return
	}
	h.clearAccessAndRefreshCookies(w, domain)

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

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, baseURL.Host, newAccessTokenID, newRefreshTokenID)

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

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, baseURL.Host, jwtToken, refreshTokenID)

	handlerutil.WriteJSONResponse(w, http.StatusOK, map[string]string{"message": "Login successful"})
}

// LinkAccount links different oauth account with same email
func (h *Handler) LinkAccount(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "linkAccount")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	linkTokenCookie, err := r.Cookie(LinkTokenCookieName)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, logger)
		return
	}

	linkTokenStr := linkTokenCookie.Value
	if linkTokenStr == "" {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, logger)
		return
	}

	provider, providerID, userIDStr, err := h.jwtIssuer.ParseLinkToken(traceCtx, linkTokenStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, logger)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, logger)
		return
	}

	// Link the new provider to the existing account
	err = h.userStore.CreateAuth(traceCtx, userID, provider, providerID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	h.clearLinkCookie(w, baseURL.Host)

	accessTokenID, refreshTokenID, err := h.generateJWT(traceCtx, userID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, baseURL.Host, accessTokenID, refreshTokenID)

	http.Redirect(w, r, "/", http.StatusFound)
}

// LinkAccountAbort aborts the merge process and logout user
func (h *Handler) LinkAccountAbort(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "linkAccountAbort")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}
	domain := baseURL.Host

	h.clearLinkCookie(w, domain)

	http.Redirect(w, r, "/", http.StatusFound)
}

// setAccessAndRefreshCookies sets the access/refresh cookies with HTTP-only and secure flags
func (h *Handler) setAccessAndRefreshCookies(w http.ResponseWriter, domain, accessTokenID, refreshTokenID string) {
	var sameSite http.SameSite
	secure := true
	if h.devMode {
		sameSite = http.SameSiteLaxMode
		domain = ""
		secure = false
	} else {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessTokenID,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Path:     "/",
		MaxAge:   int(h.accessTokenExpiration.Seconds()),
		Domain:   domain,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshTokenID,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Path:     "/",
		MaxAge:   int(h.refreshTokenExpiration.Seconds()),
		Domain:   domain,
	})
}

// clearAccessAndRefreshCookies sets the access/refresh cookies to empty values and negative MaxAge
func (h *Handler) clearAccessAndRefreshCookies(w http.ResponseWriter, domain string) {
	var sameSite http.SameSite
	secure := true
	if h.devMode {
		sameSite = http.SameSiteLaxMode
		domain = ""
		secure = false
	} else {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Domain:   domain,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Domain:   domain,
	})
}

func (h *Handler) setLinkCookie(w http.ResponseWriter, domain, tokenString string) {
	var sameSite http.SameSite
	secure := true
	if h.devMode {
		sameSite = http.SameSiteLaxMode
		domain = ""
		secure = false
	} else {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     LinkTokenCookieName,
		Value:    tokenString,
		Path:     "/",
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Domain:   domain,
	})
}

func (h *Handler) clearLinkCookie(w http.ResponseWriter, domain string) {
	var sameSite http.SameSite
	secure := true
	if h.devMode {
		sameSite = http.SameSiteLaxMode
		domain = ""
		secure = false
	} else {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     LinkTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Domain:   domain,
	})
}
