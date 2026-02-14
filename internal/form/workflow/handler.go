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

// nodeTypeToUppercase converts database node type format (lowercase) to API format (uppercase).
func nodeTypeToUppercase(nt NodeType) string {
	switch nt {
	case NodeTypeSection:
		return "SECTION"
	case NodeTypeCondition:
		return "CONDITION"
	case NodeTypeStart:
		return "START"
	case NodeTypeEnd:
		return "END"
	default:
		return string(nt)
	}
}

// nodeTypeToLowercase converts API node type format (uppercase) to database format (lowercase).
func nodeTypeToLowercase(apiType string) string {
	switch apiType {
	case "SECTION":
		return "section"
	case "CONDITION":
		return "condition"
	case "START":
		return "start"
	case "END":
		return "end"
	default:
		return strings.ToLower(apiType)
	}
}

// workflowToAPIFormat converts workflow JSON from database format to API format (type: lowercase -> uppercase).
func workflowToAPIFormat(dbWorkflow []byte) ([]byte, error) {
	var nodes []map[string]interface{}
	err := json.Unmarshal(dbWorkflow, &nodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrUnmarshalWorkflow, err)
	}

	for i := range nodes {
		if typeVal, ok := nodes[i]["type"].(string); ok {
			nodes[i]["type"] = nodeTypeToUppercase(NodeType(typeVal))
		}
	}

	result, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrMarshalWorkflow, err)
	}
	return result, nil
}

// mergeTypeFromDB merges type information from database workflow into API request.
// API request may not contain 'type' field, so we need to retrieve it from the database.
// If a node ID exists in API request but not in database, returns an error.
func mergeTypeFromDB(apiWorkflow []byte, dbWorkflow []byte) ([]byte, error) {
	// Parse API workflow (request from client)
	var apiNodes []map[string]interface{}
	err := json.Unmarshal(apiWorkflow, &apiNodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrUnmarshalAPIWorkflow, err)
	}

	// Parse database workflow
	var dbNodes []map[string]interface{}
	err = json.Unmarshal(dbWorkflow, &dbNodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrUnmarshalDBWorkflow, err)
	}

	// Build a map of node ID -> type from database
	dbNodeTypeMap := make(map[string]string)
	for _, dbNode := range dbNodes {
		nodeID, idOk := dbNode["id"].(string)
		nodeType, typeOk := dbNode["type"].(string)
		if idOk && typeOk {
			dbNodeTypeMap[nodeID] = nodeType
		}
	}

	// Merge type information into API nodes
	for i := range apiNodes {
		nodeID, ok := apiNodes[i]["id"].(string)
		if !ok || nodeID == "" {
			// Node ID is required, but let validator handle this error
			continue
		}

		// Check if node exists in database
		dbType, exists := dbNodeTypeMap[nodeID]
		if !exists {
			return nil, fmt.Errorf("%w: node with id '%s' not found in current workflow, please create it first using CreateNode API", internal.ErrWorkflowNodeNotFound, nodeID)
		}

		// Add type from database to API node
		apiNodes[i]["type"] = dbType
	}

	// Marshal merged nodes back to JSON
	result, err := json.Marshal(apiNodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrMarshalMergedWorkflow, err)
	}

	return result, nil
}

// workflowFromAPIFormat converts workflow JSON from API format to database format (type: uppercase -> lowercase).
func workflowFromAPIFormat(apiWorkflow []byte) ([]byte, error) {
	var nodes []map[string]interface{}
	err := json.Unmarshal(apiWorkflow, &nodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrUnmarshalWorkflow, err)
	}

	for i := range nodes {
		if typeVal, ok := nodes[i]["type"].(string); ok {
			nodes[i]["type"] = nodeTypeToLowercase(typeVal)
		}
	}

	result, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", internal.ErrMarshalWorkflow, err)
	}
	return result, nil
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

	// Convert workflow types from database format to API format
	apiWorkflow, err := workflowToAPIFormat([]byte(row.Workflow))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to convert workflow to API format: %w", err), logger)
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
		Workflow: json.RawMessage(apiWorkflow),
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
	req = bodyBytes

	// Get current workflow from database to merge type information
	currentRow, err := h.store.Get(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Merge type information from database into API request
	mergedWorkflow, err := mergeTypeFromDB(req, currentRow.Workflow)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to merge type information: %w", err), logger)
		return
	}

	// Convert workflow types from API format to database format
	dbWorkflow, err := workflowFromAPIFormat(mergedWorkflow)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to convert workflow from API format: %w", err), logger)
		return
	}

	row, err := h.store.Update(traceCtx, formID, dbWorkflow, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Convert response back to API format
	apiWorkflow, err := workflowToAPIFormat(row.Workflow)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to convert workflow to API format: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, json.RawMessage(apiWorkflow))
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
		Type:  nodeTypeToUppercase(created.NodeType),
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
