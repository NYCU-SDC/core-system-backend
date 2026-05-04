package workflow

import (
	"context"
	"errors"
	"fmt"
	"math"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Get(ctx context.Context, formID uuid.UUID) (WorkflowVersion, error)
	Update(ctx context.Context, arg UpdateParams) (UpdateRow, error)
	CreateNode(ctx context.Context, arg CreateNodeParams) (CreateNodeRow, error)
	DeleteNode(ctx context.Context, arg DeleteNodeParams) ([]byte, error)
	Activate(ctx context.Context, arg ActivateParams) (ActivateRow, error)
}

type Validator interface {
	Activate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore QuestionStore) error
	Validate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore QuestionStore) error
	ValidateNodeIDsUnchanged(ctx context.Context, currentWorkflow, newWorkflow []byte) error
	ValidateUpdateNodeIDs(ctx context.Context, currentWorkflow []byte, newWorkflow []byte) error
}

// NodePayload is the canonical shape for workflow node payloads.
// It is shared by the validator and other workflow code.
type NodePayload struct {
	// Use pointers so `validate:"required"` can distinguish between:
	// - field missing/null => nil pointer (invalid)
	// - valid value 0 => non-nil pointer with value 0 (valid)
	X *float64 `json:"x" validate:"required"`
	Y *float64 `json:"y" validate:"required"`
}

type Service struct {
	logger        *zap.Logger
	queries       Querier
	tracer        trace.Tracer
	validator     Validator
	questionStore QuestionStore
}

func NewService(logger *zap.Logger, db DBTX, questionStore QuestionStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("workflow/service"),
		validator:     NewValidator(),
		questionStore: questionStore,
	}
}

// NewServiceForTesting creates a Service with injected dependencies for testing.
// This allows unit tests to mock the Querier and Validator interfaces.
func NewServiceForTesting(logger *zap.Logger, tracer trace.Tracer, queries Querier, validator Validator, questionStore QuestionStore) *Service {
	return &Service{
		logger:        logger,
		queries:       queries,
		tracer:        tracer,
		validator:     validator,
		questionStore: questionStore,
	}
}

// Get retrieves the latest workflow version for a form
func (s *Service) Get(ctx context.Context, formID uuid.UUID) (WorkflowVersion, error) {
	methodName := "Get"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	workflow, err := s.queries.Get(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "get workflow by form id")
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	return workflow, nil
}

// Update updates a workflow version conditionally:
//   - If latest workflow is active and incoming workflow is structurally equal
//     (same nodes, edges, condition rules; labels ignored): returns current version
//     without creating a new one.
//   - If latest workflow is active and structure differs: creates a new workflow version.
//   - If latest workflow is draft: updates the existing workflow version.
func (s *Service) Update(ctx context.Context, formID uuid.UUID, workflow []byte, userID uuid.UUID) (WorkflowVersion, error) {
	methodName := "Update"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	if len(workflow) == 0 {
		workflow = []byte("[]")
	}

	// Validate workflow before updating
	err := s.validator.Validate(ctx, formID, workflow, s.questionStore)
	if err != nil {
		// Wrap validation error to return 400 instead of 500
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowValidationFailed, err)
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	// Get current workflow to validate node IDs haven't changed
	currentWorkflow, err := s.queries.Get(ctx, formID)
	if err != nil {
		// If workflow doesn't exist (first update), skip node ID validation
		if !errors.Is(err, pgx.ErrNoRows) {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "get current workflow")
			span.RecordError(err)
			return WorkflowVersion{}, err
		}
		// First update scenario: no existing workflow to compare against
	}

	// Extract current workflow bytes (nil if workflow doesn't exist)
	var currentWorkflowBytes []byte
	if err == nil {
		currentWorkflowBytes = currentWorkflow.Workflow
	}

	// Validate that node IDs haven't changed
	if err := s.validator.ValidateUpdateNodeIDs(ctx, currentWorkflowBytes, workflow); err != nil {
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowValidationFailed, err)
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	// When latest version is active, avoid creating a new version if only
	// display fields (e.g. labels) changed—compare structure only.
	if err == nil && currentWorkflow.IsActive {
		equal, cmpErr := structurallyEqual(currentWorkflow.Workflow, workflow)
		if cmpErr != nil {
			logger.Warn("workflow structural comparison failed", zap.Error(cmpErr))
			// Fall through to normal update
		} else if equal {
			return currentWorkflow, nil
		}
	}

	row, err := s.queries.Update(ctx, UpdateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflow,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "update workflow")
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	return WorkflowVersion(row), nil
}

func (s *Service) CreateNode(ctx context.Context, formID uuid.UUID, nodeType NodeType, payload NodePayload, userID uuid.UUID) (CreateNodeRow, error) {
	methodName := "CreateNode"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	// Validate node type
	switch nodeType {
	case NodeTypeSection:
	case NodeTypeCondition:
		break
	default:
		err := fmt.Errorf("invalid node type: %s", nodeType)
		span.RecordError(err)
		return CreateNodeRow{}, err
	}

	// Payload coordinates are required by workflow node creation.
	// We must validate before dereferencing pointers to avoid panics.
	if payload.X == nil || payload.Y == nil {
		err := fmt.Errorf("%w: payload.x and payload.y are required", internal.ErrWorkflowNodePayloadInvalid)
		span.RecordError(err)
		return CreateNodeRow{}, err
	}
	if *payload.X > math.MaxFloat64 || *payload.Y > math.MaxFloat64 {
		err := fmt.Errorf("%w: payload.x and payload.y must be int32", internal.ErrWorkflowNodePayloadInvalid)
		span.RecordError(err)
		return CreateNodeRow{}, err
	}

	createdRow, err := s.queries.CreateNode(ctx, CreateNodeParams{
		FormID:     formID,
		LastEditor: userID,
		Type:       nodeType,
		PayloadX:   float64(*payload.X),
		PayloadY:   float64(*payload.Y),
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "create node")
		span.RecordError(err)
		return CreateNodeRow{}, err
	}

	// Validate created workflow (relaxed draft validation)
	err = s.validator.Validate(ctx, formID, createdRow.Workflow, s.questionStore)
	if err != nil {
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowValidationFailed, err)
		span.RecordError(err)
		return CreateNodeRow{}, err
	}

	return createdRow, nil
}

func (s *Service) DeleteNode(ctx context.Context, formID uuid.UUID, nodeID uuid.UUID, userID uuid.UUID) ([]byte, error) {
	methodName := "DeleteNode"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	deleted, err := s.queries.DeleteNode(ctx, DeleteNodeParams{
		FormID:     formID,
		LastEditor: userID,
		NodeID:     nodeID.String(),
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "delete node")
		span.RecordError(err)
		return []byte{}, err
	}

	// Validate deleted workflow (relaxed draft validation)
	if err := s.validator.Validate(ctx, formID, deleted, s.questionStore); err != nil {
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowValidationFailed, err)
		span.RecordError(err)
		return []byte{}, err
	}

	return deleted, nil
}

func (s *Service) Activate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, workflow []byte) (WorkflowVersion, error) {
	methodName := "Activate"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	// Validate workflow before activation
	err := s.validator.Activate(ctx, formID, workflow, s.questionStore)
	if err != nil {
		// Wrap validation error to return 400 instead of 500
		err = fmt.Errorf("%w: %w", internal.ErrWorkflowValidationFailed, err)
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	row, err := s.queries.Activate(ctx, ActivateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflow,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "activate workflow")
		span.RecordError(err)
		return WorkflowVersion{}, err
	}

	return WorkflowVersion(row), nil
}

// GetValidationInfo checks if a workflow can be activated and returns detailed validation errors.
// Returns an empty slice if validation passes, or an array of ValidationInfo with node-specific errors.
func (s *Service) GetValidationInfo(ctx context.Context, formID uuid.UUID, workflow []byte) ([]ValidationInfo, error) {
	methodName := "GetValidationInfo"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()

	// Call the validator's Activate method
	err := s.validator.Activate(ctx, formID, workflow, s.questionStore)
	if err != nil {
		return parseValidationErrors(err), nil
	}

	// Activation passed; add non-blocking warnings (Activate already validated the graph).
	nodes, parseErr := parseWorkflow(workflow)
	if parseErr != nil {
		return []ValidationInfo{}, nil
	}
	err = validateAllNodesReachableFromStart(nodes)
	if err != nil {
		return parseValidationErrors(fmt.Errorf("reachability validation failed: %w", err)), nil
	}

	return []ValidationInfo{}, nil
}
