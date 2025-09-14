package jwt

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

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

		var tokenString string

		// Extract access token from cookie
		if accessTokenCookie, err := r.Cookie("access_token"); err == nil && accessTokenCookie.Value != "" {
			tokenString = accessTokenCookie.Value
		} else {
			// Fallback to Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrMissingAuthHeader, m.logger)
				return
			}

			fields := strings.Fields(authHeader)
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				m.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthHeaderFormat, m.logger)
				return
			}
			tokenString = fields[1]
		}

		// Parse and validate JWT token
		authenticatedUser, err := m.service.Parse(r.Context(), tokenString)
		if err != nil {
			m.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidAuthUser, m.logger)
			return
		}

		// Add authenticated user to request context
		ctxWithUser := context.WithValue(traceCtx, internal.UserContextKey, &authenticatedUser)

		// Call the actual handler with authenticated context
		handler(w, r.WithContext(ctxWithUser))
	}
}
