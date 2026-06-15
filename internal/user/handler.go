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

type ProfileResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatarUrl"`
	Emails    []string  `json:"emails"`
}

// MeResponse represents the response format for /user/me endpoint.
type MeResponse struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Name      string   `json:"name"`
	AvatarUrl string   `json:"avatarUrl"`
	Role      string   `json:"role"`
	Emails    []string `json:"emails"`

	// Todo: This field is currently always false, but we keep it here for future use when we want to enforce onboarding for invited users
	RequireOnboarding bool `json:"require_onboarding"`
}

// OnboardingRequest represents the request format for /user/onboarding endpoint
type OnboardingRequest struct {
	Username string `json:"username" validate:"required,min=4,max=15,username_rules"`
	Name     string `json:"name" validate:"required,max=15"`
}

type Store interface {
	Get(ctx context.Context, id uuid.UUID) (UserDetail, error)
	Onboarding(ctx context.Context, id uuid.UUID, name, username string) (User, error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	tracer        trace.Tracer
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
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

	user, err := h.store.Get(traceCtx, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	meResponse := MeResponse{
		ID:                user.ID.String(),
		Username:          user.Username,
		Name:              user.Name,
		AvatarUrl:         user.AvatarURL,
		Role:              ConvertRoleToString(user.Role),
		Emails:            user.Emails,
		RequireOnboarding: false,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, meResponse)
}

// Onboarding handles PUT /users/onboarding - update the user's name and username
func (h *Handler) Onboarding(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Onboarding")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req OnboardingRequest
	err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrValidationFailed, logger)
		return
	}

	// Get authenticated user from context
	currentUser, ok := GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Onboarding
	newUser, err := h.store.Onboarding(traceCtx, currentUser.ID, req.Name, req.Username)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	user, err := h.store.Get(traceCtx, newUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	meResponse := MeResponse{
		ID:                user.ID.String(),
		Username:          user.Username,
		Name:              user.Name,
		AvatarUrl:         user.AvatarURL,
		Role:              ConvertRoleToString(user.Role),
		Emails:            user.Emails,
		RequireOnboarding: false,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, meResponse)
}

func ConvertEmailsToSlice(emails interface{}) []string {
	if emails == nil {
		return []string{}
	}

	switch v := emails.(type) {
	case []string:
		if v == nil {
			return []string{}
		}
		return v
	case []interface{}:
		// Handle PostgreSQL array returned as []interface{}
		result := make([]string, 0, len(v))
		for _, email := range v {
			if str, ok := email.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return []string{}
	}
}

func ConvertRoleToString(roles []string) string {
	if roles == nil {
		return ""
	}

	return strings.Join(roles, ",")
}
