package workflow

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type Store interface {
	Get(ctx context.Context, formID uuid.UUID) (GetRow, error)
	Update(ctx context.Context, formID uuid.UUID, workflow []byte, userID uuid.UUID) (UpdateRow, error)
	CreateNode(ctx context.Context, formID uuid.UUID, nodeType NodeType, userID uuid.UUID) (CreateNodeRow, error)
	DeleteNode(ctx context.Context, formID uuid.UUID, nodeID uuid.UUID, userID uuid.UUID) ([]byte, error)
	Activate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, workflow []byte) (ActivateRow, error)
	GetValidationInfo(ctx context.Context, formID uuid.UUID, workflow []byte) ([]ValidationInfo, error)
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store Store
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("workflow/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
	}
}

type createNodeRequest struct {
	Type string `json:"type" validate:"required,oneof=SECTION CONDITION"`
}

type createNodeResponse struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"`
	Label interface{} `json:"label"`
}

type ValidationInfo struct {
	Type    ValidationInfoType `json:"type"`
	NodeID  *string            `json:"nodeId,omitempty"`
	Message string             `json:"message"`
}

type GetWorkflowResponse struct {
	Workflow json.RawMessage  `json:"workflow"`
	Info     []ValidationInfo `json:"info"`
}

func (h *Handler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetWorkflow")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("id")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	row, err := h.store.Get(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Check validation status
	validationInfos, err := h.store.GetValidationInfo(traceCtx, formID, []byte(row.Workflow))
	if err != nil {
		// Log the error but don't fail the request - return empty info array
		logger.Warn("failed to validate workflow activation", zap.Error(err), zap.String("formId", formID.String()))
		validationInfos = []ValidationInfo{}
	}

	response := GetWorkflowResponse{
		Workflow: json.RawMessage(row.Workflow),
		Info:     validationInfos,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateWorkflow")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("id")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Read request body as json.RawMessage
	// json.RawMessage doesn't need struct validation, so read body directly
	var req json.RawMessage
	if r.Body == nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("request body is nil"), logger)
		return
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to read request body: %w", err), logger)
		return
	}
	if len(bodyBytes) == 0 {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("request body is empty"), logger)
		return
	}

	var unmarshalTest interface{}
	err = json.Unmarshal(bodyBytes, &unmarshalTest)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid JSON in request body: %w", err), logger)
		return
	}
	req = json.RawMessage(bodyBytes)

	row, err := h.store.Update(traceCtx, formID, []byte(req), currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, json.RawMessage(row.Workflow))
}

func (h *Handler) CreateNode(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateNode")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req createNodeRequest
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Convert uppercase request value to lowercase for database storage
	nodeType := NodeType(strings.ToLower(req.Type))
	created, err := h.store.CreateNode(traceCtx, formID, nodeType, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, createNodeResponse{
		ID:    created.NodeID.String(),
		Type:  string(created.NodeType),
		Label: created.NodeLabel,
	})
}

func (h *Handler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteNode")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	nodeIDStr := r.PathValue("nodeId")
	nodeID, err := handlerutil.ParseUUID(nodeIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	_, err = h.store.DeleteNode(traceCtx, formID, nodeID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}
