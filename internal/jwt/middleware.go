package jwt

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Middleware struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	service       *Service
	tracer        trace.Tracer
}

// NewMiddleware creates a new user middleware
func NewMiddleware(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	service *Service,
) *Middleware {
	return &Middleware{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		service:       service,
		tracer:        otel.Tracer("jwt/middleware"),
	}
}

// AuthenticateMiddleware validates JWT token and adds user to context
func (m *Middleware) AuthenticateMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceCtx, span := m.tracer.Start(r.Context(), "AuthenticateMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		var tokenString string

		// Extract access token from cookie
		if accessTokenCookie, err := r.Cookie("access_token"); err == nil && accessTokenCookie.Value != "" {
			tokenString = accessTokenCookie.Value
		} else {
			// Fallback to Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, logger)
				return
			}

			fields := strings.Fields(authHeader)
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, logger)
				return
			}
			tokenString = fields[1]
		}

		// Parse and validate JWT token
		authenticatedUser, err := m.service.Parse(r.Context(), tokenString)
		if err != nil {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthUser, logger)
			return
		}

		// Add authenticated user to request context
		ctxWithUser := context.WithValue(traceCtx, internal.UserContextKey, &authenticatedUser)

		// Call the actual handler with authenticated context
		handler(w, r.WithContext(ctxWithUser))
	}
}

// OptionalAuthMiddleware validates JWT token if present, but allows requests without token
// If token is present and valid, user is added to context. If not present, request continues without user context.
func (m *Middleware) OptionalAuthMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceCtx, span := m.tracer.Start(r.Context(), "OptionalAuthMiddleware")
		defer span.End()
		logger := logutil.WithContext(traceCtx, m.logger)

		var tokenString string

		// Extract access token from cookie
		if accessTokenCookie, err := r.Cookie("access_token"); err == nil && accessTokenCookie.Value != "" {
			tokenString = accessTokenCookie.Value
		} else {
			// Fallback to Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				fields := strings.Fields(authHeader)
				if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
					tokenString = fields[1]
				}
			}
		}

		// If no token found, continue without user context
		if tokenString == "" {
			logger.Debug("No authentication token found, continuing without user context")
			handler(w, r.WithContext(traceCtx))
			return
		}

		// Parse and validate JWT token
		authenticatedUser, err := m.service.Parse(r.Context(), tokenString)
		if err != nil {
			// Invalid token, but we still allow the request to continue without user context
			logger.Debug("Invalid token provided, continuing without user context", zap.Error(err))
			handler(w, r.WithContext(traceCtx))
			return
		}

		// Add authenticated user to request context
		ctxWithUser := context.WithValue(traceCtx, internal.UserContextKey, &authenticatedUser)

		// Call the actual handler with authenticated context
		handler(w, r.WithContext(ctxWithUser))
	}
}
