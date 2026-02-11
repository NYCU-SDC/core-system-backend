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

func ConvertEmailsToSlice(emails interface{}) []string {
	switch v := emails.(type) {
	case []string:
		if v == nil {
			return []string{}
		}
		return v
	default:
		return []string{}
	}
}

type ProfileResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatarUrl"`
	Emails    []string  `json:"emails"`
}

// MeResponse represents the response format for /user/me endpoint
type MeResponse struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Name      string   `json:"name"`
	AvatarUrl string   `json:"avatarUrl"`
	Role      string   `json:"role"`
	Emails    []string `json:"emails"`
}

// OnboardingRequest represents the request format for /user/onboarding endpoint
type OnboardingRequest struct {
	Username string `json:"username" validate:"required,min=4,max=15,username_rules"`
	Name     string `json:"name" validate:"required,max=15"`
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

	// Get user emails
	emails, err := h.service.GetEmailsByID(traceCtx, currentUser.ID)
	if err != nil {
		logger.Warn("Failed to get user emails", zap.Error(err), zap.String("user_id", currentUser.ID.String()))
		emails = []string{}
	}

	response := MeResponse{
		ID:        currentUser.ID.String(),
		Username:  currentUser.Username.String,
		Name:      currentUser.Name.String,
		AvatarUrl: currentUser.AvatarUrl.String,
		Role:      roleStr,
		Emails:    emails,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// Onboarding handles PUT /users/onboarding - update the user's name and username
func (h *Handler) Onboarding(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "Onboarding")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req OnboardingRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrValidationFailed, logger)
		return
	}

	// Get authenticated userfrom context
	currentUser, ok := GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Onboarding
	newUser, err := h.service.Onboarding(traceCtx, currentUser.ID, req.Name, req.Username)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Convert roles array to comma-separated string
	roleStr := ""
	if len(newUser.Role) > 0 {
		roleStr = strings.Join(newUser.Role, ",")
	}

	// Get user emails
	emails, err := h.service.GetEmailsByID(traceCtx, newUser.ID)
	if err != nil {
		logger.Warn("Failed to get user emails", zap.Error(err), zap.String("user_id", newUser.ID.String()))
		emails = []string{}
	}

	response := MeResponse{
		ID:        newUser.ID.String(),
		Username:  newUser.Username.String,
		Name:      newUser.Name.String,
		AvatarUrl: newUser.AvatarUrl.String,
		Role:      roleStr,
		Emails:    emails,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}
