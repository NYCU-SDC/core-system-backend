package user

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"
	"strings"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// GetFromContext extracts the authenticated user from request context
func GetFromContext(ctx context.Context) (*User, bool) {
	userData, ok := ctx.Value(internal.UserContextKey).(*User)
	return userData, ok
}

type ProfileResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	Email     []string  `json:"email"`
	AvatarURL string    `json:"avatarUrl"`
}

// MeResponse represents the response format for /user/me endpoint
type MeResponse struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Email     []string `json:"email"`
	Name      string   `json:"name"`
	AvatarUrl string   `json:"avatarUrl"`
	Role      string   `json:"role"`
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	service       *Service
	tracer        trace.Tracer
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	service *Service,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		service:       service,
		tracer:        otel.Tracer("user/handler"),
	}
}

// GetMe handles GET /user/me - returns authenticated user information
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetMe")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get authenticated user from context
	currentUser, ok := GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Convert roles array to comma-separated string
	roleStr := ""
	if len(currentUser.Role) > 0 {
		roleStr = strings.Join(currentUser.Role, ",")
	}

	response := MeResponse{
		ID:        currentUser.ID.String(),
		Username:  currentUser.Username.String,
		Email:     currentUser.Email,
		Name:      currentUser.Name.String,
		AvatarUrl: currentUser.AvatarUrl.String,
		Role:      roleStr,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}
