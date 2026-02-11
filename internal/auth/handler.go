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
	NewState(ctx context.Context, service, environment, callbackURL, redirectURL string) (string, error)
	Parse(ctx context.Context, tokenString string) (user.User, error)
	ParseState(ctx context.Context, tokenString string) (string, error)
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
	FindOrCreate(ctx context.Context, name, username, avatarUrl string, role []string, oauthProvider, oauthProviderID string) (uuid.UUID, error)
	CreateEmail(ctx context.Context, userID uuid.UUID, email string) error
}

type OAuthProvider interface {
	Name() string
	Config() *oauth2.Config
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, token *oauth2.Token) (user.User, user.Auth, string, error)
}

type CallBackInfo struct {
	code       string
	oauthError string
	redirectTo string
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
	providers map[string]OAuthProvider,

	baseURL string,
	oauthProxyBaseURL string,
	environment string,
	devMode bool,

	accessTokenExpiration time.Duration,
	refreshTokenExpiration time.Duration,
) *Handler {
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
		provider:  providers,

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

	redirectURL := r.URL.Query().Get("r")

	// Determine callback URL based on oauth proxy configuration
	callbackURL := ""
	if h.oauthProxyBaseURL != "" {
		callbackURL = fmt.Sprintf("%s/api/auth/login/oauth/%s/callback", h.baseURL, providerName)
	}

	// Create JWT state for OAuth flow
	state, err := h.jwtIssuer.NewState(traceCtx, "core-system", h.environment, callbackURL, redirectURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrNewStateFailed, err), logger)
		return
	}

	// Generate OAuth authorization URL and redirect
	authURL := provider.Config().AuthCodeURL(state, oauth2.AccessTypeOffline)
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
	callbackInfo, err := h.GetCallBackInfo(traceCtx, r.URL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidCallbackInfo, err), logger)
		return
	}

	code := callbackInfo.code
	redirectTo := callbackInfo.redirectTo
	oauthError := callbackInfo.oauthError

	if oauthError != "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %s", internal.ErrOAuthError, oauthError), logger)
		return
	}

	token, err := provider.Exchange(traceCtx, code)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrInvalidExchangeToken, err), logger)
		return
	}

	userInfo, auth, email, err := provider.GetUserInfo(traceCtx, token)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	userID, err := h.userStore.FindOrCreate(traceCtx, userInfo.Name.String, userInfo.Username.String, userInfo.AvatarUrl.String, userInfo.Role, providerName, auth.ProviderID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Create email record for OAuth users if email is available
	if email != "" {
		err := h.userStore.CreateEmail(traceCtx, userID, email)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, internal.ErrFailedToCreateEmail, logger)
			return
		}
	}

	accessTokenID, refreshTokenID, err := h.generateJWT(traceCtx, userID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	baseURL, err := url.Parse(h.baseURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	h.setAccessAndRefreshCookies(w, baseURL.Host, accessTokenID, refreshTokenID)

	redirectURL := redirectTo
	if redirectURL == "" {
		// If environment is "snapshot" or "no-env", meaning it should have no frontend
		// redirect to the API endpoint, otherwise redirect to the home page
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

func (h *Handler) GetCallBackInfo(ctx context.Context, url *url.URL) (CallBackInfo, error) {
	code := url.Query().Get("code")
	state := url.Query().Get("state")
	oauthError := url.Query().Get("error")

	redirectURL, err := h.jwtIssuer.ParseState(ctx, state)
	if err != nil {
		return CallBackInfo{}, err
	}

	return CallBackInfo{
		code:       code,
		oauthError: oauthError,
		redirectTo: redirectURL,
	}, nil
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Logout")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Inactivate the current refresh token from cookie
	refreshTokenCookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil {
		logger.Error("Failed to get refresh token cookie during logout", zap.Error(err))
		return
	}

	refreshTokenID, err := uuid.Parse(refreshTokenCookie.Value)
	if err != nil {
		logger.Error("Invalid refresh token format during logout", zap.Error(err))
		return
	}

	err = h.jwtStore.InactivateRefreshToken(traceCtx, refreshTokenID)
	if err != nil {
		logger.Warn("Failed to inactivate refresh token during logout", zap.Error(err))
		return
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

// setAccessAndRefreshCookies sets the access/refresh cookies with HTTP-only and secure flags
func (h *Handler) setAccessAndRefreshCookies(w http.ResponseWriter, domain, accessTokenID, refreshTokenID string) {
	var sameSite http.SameSite
	if h.devMode {
		sameSite = http.SameSiteNoneMode
	} else {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessTokenID,
		HttpOnly: true,
		Secure:   true,
		SameSite: sameSite,
		Path:     "/",
		MaxAge:   int(h.accessTokenExpiration.Seconds()),
		Domain:   domain,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshTokenID,
		HttpOnly: true,
		Secure:   true,
		SameSite: sameSite,
		Path:     "/api/auth/refresh",
		MaxAge:   int(h.refreshTokenExpiration.Seconds()),
		Domain:   domain,
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

// CreateOAuthProvider creates a single OAuth provider with a specific callback URL
func CreateOAuthProvider(
	providerName string,
	callbackURL string,
	googleOauthConfig oauthprovider.GoogleOauth,
	githubOauthConfig oauthprovider.GitHubOauth,
) OAuthProvider {
	switch providerName {
	case "google":
		if googleOauthConfig.ClientID != "" && googleOauthConfig.ClientSecret != "" {
			return oauthprovider.NewGoogleConfig(
				googleOauthConfig.ClientID,
				googleOauthConfig.ClientSecret,
				callbackURL,
			)
		}
	case "github":
		if githubOauthConfig.ClientID != "" && githubOauthConfig.ClientSecret != "" {
			return oauthprovider.NewGitHubConfig(
				githubOauthConfig.ClientID,
				githubOauthConfig.ClientSecret,
				callbackURL,
			)
		}
	}
	return nil
}

// CreateAuthProviders creates OAuth providers for user authentication (login)
func CreateAuthProviders(
	logger *zap.Logger,
	baseURL string,
	oauthProxyBaseURL string,
	googleOauthConfig oauthprovider.GoogleOauth,
	githubOauthConfig oauthprovider.GitHubOauth,
) map[string]OAuthProvider {
	providers := make(map[string]OAuthProvider)

	// Google OAuth configuration for authentication
	var googleCallbackURL string
	if oauthProxyBaseURL != "" {
		googleCallbackURL = fmt.Sprintf("%s/api/auth/login/oauth/google/callback", oauthProxyBaseURL)
	} else {
		googleCallbackURL = fmt.Sprintf("%s/api/auth/login/oauth/google/callback", baseURL)
	}

	if provider := CreateOAuthProvider("google", googleCallbackURL, googleOauthConfig, githubOauthConfig); provider != nil {
		providers["google"] = provider
		logger.Info("Google OAuth provider configured for auth", zap.String("callbackURL", googleCallbackURL))
	}

	if len(providers) == 0 {
		logger.Warn("No OAuth providers configured for authentication")
	}

	return providers
}

// CreateQuestionOAuthProviders creates OAuth providers for form questions
func CreateQuestionOAuthProviders(
	logger *zap.Logger,
	baseURL string,
	googleOauthConfig oauthprovider.GoogleOauth,
	githubOauthConfig oauthprovider.GitHubOauth,
) map[string]OAuthProvider {
	providers := make(map[string]OAuthProvider)

	// Google OAuth configuration for questions
	googleCallbackURL := fmt.Sprintf("%s/api/oauth/questions/google/callback", baseURL)
	if provider := CreateOAuthProvider("google", googleCallbackURL, googleOauthConfig, githubOauthConfig); provider != nil {
		providers["google"] = provider
		logger.Info("Google OAuth provider configured for questions", zap.String("callbackURL", googleCallbackURL))
	}

	// GitHub OAuth configuration for questions
	githubCallbackURL := fmt.Sprintf("%s/api/oauth/questions/github/callback", baseURL)
	if provider := CreateOAuthProvider("github", githubCallbackURL, googleOauthConfig, githubOauthConfig); provider != nil {
		providers["github"] = provider
		logger.Info("GitHub OAuth provider configured for questions", zap.String("callbackURL", githubCallbackURL))
	}

	if len(providers) == 0 {
		logger.Warn("No OAuth providers configured for questions")
	}

	return providers
}
