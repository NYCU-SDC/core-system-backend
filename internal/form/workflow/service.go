package workflow

import (
	"context"
	"errors"
	"fmt"

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
	Get(ctx context.Context, formID uuid.UUID) (GetRow, error)
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

type Service struct {
	logger        *zap.Logger
	queries       Querier
	tracer        trace.Tracer
	validator     Validator
	questionStore QuestionStore
}

func NewService(logger *zap.Logger, db DBTX, questionService QuestionStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("workflow/service"),
		validator:     NewValidator(),
		questionStore: questionService,
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
func (s *Service) Get(ctx context.Context, formID uuid.UUID) (GetRow, error) {
	methodName := "Get"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	workflow, err := s.queries.Get(ctx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "get workflow by form id")
		span.RecordError(err)
		return GetRow{}, err
	}

	return workflow, nil
}

// Update updates a workflow version conditionally:
// - If latest workflow is active: creates a new workflow version
// - If latest workflow is draft: updates the existing workflow version
func (s *Service) Update(ctx context.Context, formID uuid.UUID, workflow []byte, userID uuid.UUID) (UpdateRow, error) {
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
		return UpdateRow{}, err
	}

	// Get current workflow to validate node IDs haven't changed
	currentWorkflow, err := s.queries.Get(ctx, formID)
	if err != nil {
		// If workflow doesn't exist (first update), skip node ID validation
		if !errors.Is(err, pgx.ErrNoRows) {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "get current workflow")
			span.RecordError(err)
			return UpdateRow{}, err
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
		return UpdateRow{}, err
	}

	updated, err := s.queries.Update(ctx, UpdateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflow,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "update workflow")
		span.RecordError(err)
		return UpdateRow{}, err
	}

	return updated, nil
}

func (s *Service) CreateNode(ctx context.Context, formID uuid.UUID, nodeType NodeType, userID uuid.UUID) (CreateNodeRow, error) {
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

	createdRow, err := s.queries.CreateNode(ctx, CreateNodeParams{
		FormID:     formID,
		LastEditor: userID,
		Type:       nodeType,
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

func (s *Service) Activate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, workflow []byte) (ActivateRow, error) {
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
		return ActivateRow{}, err
	}

	activatedVersion, err := s.queries.Activate(ctx, ActivateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflow,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "workflow", "formId", formID.String(), logger, "activate workflow")
		span.RecordError(err)
		return ActivateRow{}, err
	}

	return activatedVersion, nil
}

// GetValidationInfo checks if a workflow can be activated and returns detailed validation errors.
// Returns an empty slice if validation passes, or an array of ValidationInfo with node-specific errors.
func (s *Service) GetValidationInfo(ctx context.Context, formID uuid.UUID, workflow []byte) ([]ValidationInfo, error) {
	methodName := "GetValidationInfo"
	ctx, span := s.tracer.Start(ctx, methodName)
	defer span.End()

	// Call the validator's Activate method
	err := s.validator.Activate(ctx, formID, workflow, s.questionStore)
	if err == nil {
		// Validation passed
		return []ValidationInfo{}, nil
	}

	// Parse the validation errors
	validationInfos := parseValidationErrors(err)
	return validationInfos, nil
}
