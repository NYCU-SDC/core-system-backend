package jwt

import (
	"NYCU-SDC/core-system-backend/internal/user"

	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/golang-jwt/jwt/v5"
)

const Issuer = "core-system"

type Querier interface {
	GetUserIDByTokenID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	Create(ctx context.Context, arg CreateParams) (RefreshToken, error)
	Inactivate(ctx context.Context, id uuid.UUID) (int64, error)
	Delete(ctx context.Context) (int64, error)
	GetRefreshTokenByID(ctx context.Context, id uuid.UUID) (RefreshToken, error)
}

type Service struct {
	logger                 *zap.Logger
	secret                 string
	oauthProxySecret       string
	accessTokenExpiration  time.Duration
	refreshTokenExpiration time.Duration
	queries                Querier
	tracer                 trace.Tracer
}

func NewService(
	logger *zap.Logger,
	db DBTX,
	secret string,
	oauthProxySecret string,
	accessTokenExpiration time.Duration,
	refreshTokenExpiration time.Duration,
) *Service {
	return &Service{
		logger:                 logger,
		queries:                New(db),
		tracer:                 otel.Tracer("jwt/service"),
		secret:                 secret,
		oauthProxySecret:       oauthProxySecret,
		accessTokenExpiration:  accessTokenExpiration,
		refreshTokenExpiration: refreshTokenExpiration,
	}
}

type claims struct {
	ID        uuid.UUID
	Username  string
	Name      string
	AvatarUrl string
	Role      []string
	jwt.RegisteredClaims
}

// oauthProxyClaims defines contextual information for an OAuth transaction.
// It is encoded into the 'state' parameter as a signed JWT to preserve integrity and authenticity.
type oauthProxyClaims struct {
	// Service is the logical service requesting authentication (e.g., "core-system", "clustron").
	Service string

	// Environment represents the environment or deployment context (e.g., "pr-12", "staging").
	Environment string

	// CallbackURL is the backend endpoint to receive the OAuth authorization code.
	// It must be an internal service endpoint, not exposed to users.
	CallbackURL string

	// RedirectURL is the final URL to send the user to after authentication completes.
	// This is typically a user-facing frontend page.
	RedirectURL string

	jwt.RegisteredClaims
}

// oauthFormClaims extends oauthProxyClaims with form-specific context needed for
// OAuth-connected form questions. It carries the response and question IDs so that
// the callback handler can store the OAuth result as a form answer.
type oauthFormClaims struct {
	// CallbackURL is the backend endpoint to receive the OAuth authorization code.
	CallbackURL string

	// ResponseID is the UUID of the form response being filled in.
	ResponseID string

	// QuestionID is the UUID of the oauth_connect question being answered.
	QuestionID string

	// RedirectURL is the final URL to send the user to after the OAuth flow completes.
	RedirectURL string

	jwt.RegisteredClaims
}

func (s Service) New(ctx context.Context, user user.User) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "New")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	jwtID := uuid.New()

	id := user.ID
	username := user.Username.String

	claims := &claims{
		ID:        jwtID,
		Username:  username,
		Name:      user.Name.String,
		AvatarUrl: user.AvatarUrl.String,
		Role:      user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   id.String(), // user id
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ID:        jwtID.String(), // jwt id
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.secret))
	if err != nil {
		logger.Error("failed to sign token", zap.Error(err), zap.String("user_id", id.String()), zap.String("username", username), zap.String("role", strings.Join(user.Role, ",")))
		return "", err
	}

	logger.Debug("Generated JWT token", zap.String("id", id.String()), zap.String("username", username), zap.String("role", strings.Join(user.Role, ",")))
	return tokenString, nil
}

func (s Service) NewState(ctx context.Context, service, environment, callbackURL, redirectURL string) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "NewState")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	id := uuid.New()
	claims := &oauthProxyClaims{
		Service:     service,
		Environment: environment,
		CallbackURL: callbackURL,
		RedirectURL: redirectURL,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   id.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now()),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        id.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.oauthProxySecret))
	if err != nil {
		logger.Error("failed to sign state token", zap.Error(err), zap.String("service", service), zap.String("environment", environment))
		return "", err
	}

	logger.Debug("Generated OAuth proxy state token", zap.String("service", service), zap.String("environment", environment))
	return tokenString, nil
}

func (s Service) Parse(ctx context.Context, tokenString string) (user.User, error) {
	traceCtx, span := s.tracer.Start(ctx, "Parse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	secret := func(token *jwt.Token) (interface{}, error) {
		return []byte(s.secret), nil
	}

	tokenClaims := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, tokenClaims, secret)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenMalformed):
			logger.Warn("Failed to parse JWT token due to malformed structure, this is not a JWT token", zap.String("token", tokenString), zap.String("error", err.Error()))
			return user.User{}, err
		case errors.Is(err, jwt.ErrSignatureInvalid):
			logger.Warn("Failed to parse JWT token due to invalid signature", zap.String("error", err.Error()))
			return user.User{}, err
		case errors.Is(err, jwt.ErrTokenExpired):
			expiredTime, getErr := token.Claims.GetExpirationTime()
			if getErr != nil {
				logger.Error("Failed to parse JWT token due to expired timestamp", zap.String("error", getErr.Error()))
				return user.User{}, err
			}
			logger.Warn("Failed to parse JWT token due to expired timestamp", zap.String("error", err.Error()), zap.Time("expired_at", expiredTime.Time))
			return user.User{}, err
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			notBeforeTime, getErr := token.Claims.GetNotBefore()
			if getErr != nil {
				logger.Error("Failed to parse JWT token due to not valid yet timestamp", zap.String("error", getErr.Error()))
				return user.User{}, err
			}
			logger.Warn("Failed to parse JWT token due to not valid yet timestamp", zap.String("error", err.Error()), zap.Time("not_before", notBeforeTime.Time))
			return user.User{}, err
		default:
			logger.Error("Failed to parse JWT token", zap.Error(err))
			return user.User{}, err
		}
	}

	// Parse user ID from subject
	userID, err := uuid.Parse(tokenClaims.Subject)
	if err != nil {
		logger.Error("Failed to parse user ID from JWT subject", zap.Error(err))
		return user.User{}, err
	}

	return user.User{
		ID:        userID,
		Username:  pgtype.Text{String: tokenClaims.Username, Valid: true},
		Name:      pgtype.Text{String: tokenClaims.Name, Valid: true},
		AvatarUrl: pgtype.Text{String: tokenClaims.AvatarUrl, Valid: true},
		Role:      tokenClaims.Role,
	}, nil
}

// ParseState parses the state jwt payload to get redirect URL
func (s Service) ParseState(ctx context.Context, tokenString string) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "ParseState")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	secret := func(token *jwt.Token) (interface{}, error) {
		return []byte(s.oauthProxySecret), nil
	}

	tokenClaims := &oauthProxyClaims{}
	token, err := jwt.ParseWithClaims(tokenString, tokenClaims, secret)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenMalformed):
			logger.Warn("Failed to parse JWT token due to malformed structure, this is not a JWT token", zap.String("token", tokenString), zap.String("error", err.Error()))
			return "", err
		case errors.Is(err, jwt.ErrSignatureInvalid):
			logger.Warn("Failed to parse JWT token due to invalid signature", zap.String("error", err.Error()))
			return "", err
		case errors.Is(err, jwt.ErrTokenExpired):
			expiredTime, getErr := token.Claims.GetExpirationTime()
			if getErr != nil {
				logger.Error("Failed to parse JWT token due to expired timestamp", zap.String("error", getErr.Error()))
				return "", err
			}
			logger.Warn("Failed to parse JWT token due to expired timestamp", zap.String("error", err.Error()), zap.Time("expired_at", expiredTime.Time))
			return "", err
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			notBeforeTime, getErr := token.Claims.GetNotBefore()
			if getErr != nil {
				logger.Error("Failed to parse JWT token due to not valid yet timestamp", zap.String("error", getErr.Error()))
				return "", err
			}
			logger.Warn("Failed to parse JWT token due to not valid yet timestamp", zap.String("error", err.Error()), zap.Time("not_before", notBeforeTime.Time))
			return "", err
		default:
			logger.Error("Failed to parse JWT token", zap.Error(err))
			return "", err
		}
	}

	logger.Debug("Successfully parsed OAuth proxy state token", zap.String("service", tokenClaims.Service), zap.String("environment", tokenClaims.Environment), zap.String("callback_url", tokenClaims.CallbackURL), zap.String("redirect_url", tokenClaims.RedirectURL))

	return tokenClaims.RedirectURL, nil
}

// NewFormState creates a signed JWT to be used as the OAuth state parameter for form-question OAuth flows.
// The token encodes the callbackURL, responseID, questionID, and optional redirectURL.
func (s Service) NewFormState(ctx context.Context, callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string) (string, error) {
	traceCtx, span := s.tracer.Start(ctx, "NewFormState")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	id := uuid.New()
	claims := &oauthFormClaims{
		CallbackURL: callbackURL,
		ResponseID:  responseID.String(),
		QuestionID:  questionID.String(),
		RedirectURL: redirectURL,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   id.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now()),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        id.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.oauthProxySecret))
	if err != nil {
		logger.Error("failed to sign form state token", zap.Error(err), zap.String("response_id", responseID.String()), zap.String("question_id", questionID.String()))
		return "", err
	}

	logger.Debug("Generated OAuth form state token", zap.String("response_id", responseID.String()), zap.String("question_id", questionID.String()))
	return tokenString, nil
}

// ParseFormState parses a form-question OAuth state JWT and returns its contents.
func (s Service) ParseFormState(ctx context.Context, tokenString string) (callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string, err error) {
	traceCtx, span := s.tracer.Start(ctx, "ParseFormState")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	secret := func(token *jwt.Token) (interface{}, error) {
		return []byte(s.oauthProxySecret), nil
	}

	tokenClaims := &oauthFormClaims{}
	token, parseErr := jwt.ParseWithClaims(tokenString, tokenClaims, secret)
	if parseErr != nil {
		switch {
		case errors.Is(parseErr, jwt.ErrTokenMalformed):
			logger.Warn("Failed to parse form state token due to malformed structure", zap.String("error", parseErr.Error()))
			return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
		case errors.Is(parseErr, jwt.ErrSignatureInvalid):
			logger.Warn("Failed to parse form state token due to invalid signature", zap.String("error", parseErr.Error()))
			return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
		case errors.Is(parseErr, jwt.ErrTokenExpired):
			expiredTime, getErr := token.Claims.GetExpirationTime()
			if getErr != nil {
				logger.Error("Failed to parse form state token due to expired timestamp", zap.String("error", getErr.Error()))
				return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
			}
			logger.Warn("Failed to parse form state token due to expired timestamp", zap.String("error", parseErr.Error()), zap.Time("expired_at", expiredTime.Time))
			return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
		case errors.Is(parseErr, jwt.ErrTokenNotValidYet):
			notBeforeTime, getErr := token.Claims.GetNotBefore()
			if getErr != nil {
				logger.Error("Failed to parse form state token due to not valid yet timestamp", zap.String("error", getErr.Error()))
				return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
			}
			logger.Warn("Failed to parse form state token due to not valid yet timestamp", zap.String("error", parseErr.Error()), zap.Time("not_before", notBeforeTime.Time))
			return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
		default:
			logger.Error("Failed to parse form state token", zap.Error(parseErr))
			return "", uuid.UUID{}, uuid.UUID{}, "", parseErr
		}
	}

	parsedResponseID, err := uuid.Parse(tokenClaims.ResponseID)
	if err != nil {
		logger.Error("Failed to parse response_id from form state token", zap.String("response_id", tokenClaims.ResponseID), zap.Error(err))
		return "", uuid.UUID{}, uuid.UUID{}, "", err
	}

	parsedQuestionID, err := uuid.Parse(tokenClaims.QuestionID)
	if err != nil {
		logger.Error("Failed to parse question_id from form state token", zap.String("question_id", tokenClaims.QuestionID), zap.Error(err))
		return "", uuid.UUID{}, uuid.UUID{}, "", err
	}

	logger.Debug("Successfully parsed OAuth form state token",
		zap.String("response_id", tokenClaims.ResponseID),
		zap.String("question_id", tokenClaims.QuestionID),
		zap.String("callback_url", tokenClaims.CallbackURL),
	)
	return tokenClaims.CallbackURL, parsedResponseID, parsedQuestionID, tokenClaims.RedirectURL, nil
}

func (s Service) GetUserIDByRefreshToken(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetUserIDByRefreshToken")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	userID, err := s.queries.GetUserIDByTokenID(ctx, id)
	if err != nil {
		logger.Error("failed to get user id by refresh token", zap.Error(err))
		return uuid.UUID{}, err
	}

	return userID, nil
}

func (s Service) GenerateRefreshToken(ctx context.Context, userID uuid.UUID) (RefreshToken, error) {
	traceCtx, span := s.tracer.Start(ctx, "GenerateRefreshToken")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	rowsAffected, err := s.DeleteExpiredRefreshTokens(traceCtx)
	if err != nil {
		logger.Error("failed to delete expired refresh tokens", zap.Error(err))
	}
	if rowsAffected > 0 {
		logger.Info("deleted expired refresh tokens", zap.Int64("rows_affected", rowsAffected))
	}

	expirationDate := time.Now()
	nextRefreshDate := expirationDate.Add(s.refreshTokenExpiration)

	params := CreateParams{
		UserID: userID,
		ExpirationDate: pgtype.Timestamptz{
			Time:  nextRefreshDate,
			Valid: true,
		},
	}
	refreshToken, err := s.queries.Create(traceCtx, params)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "generate refresh token")
		span.RecordError(err)
		return RefreshToken{}, err
	}
	return refreshToken, nil
}

func (s Service) InactivateRefreshToken(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "InactivateRefreshToken")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	_, err := s.queries.Inactivate(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "refresh_token", "id", id.String(), logger, "inactivate refresh token")
		return err
	}

	return nil
}

func (s Service) DeleteExpiredRefreshTokens(ctx context.Context) (int64, error) {
	traceCtx, span := s.tracer.Start(ctx, "DeleteExpiredRefreshTokens")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	rowsAffected, err := s.queries.Delete(traceCtx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "delete expired refresh tokens")
		span.RecordError(err)
		return 0, err
	}

	return rowsAffected, nil
}

func (s Service) GetRefreshTokenByID(ctx context.Context, id uuid.UUID) (RefreshToken, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetRefreshTokenByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	refreshToken, err := s.queries.GetRefreshTokenByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get refresh token by id")
		span.RecordError(err)
		return RefreshToken{}, err
	}

	return refreshToken, nil
}
