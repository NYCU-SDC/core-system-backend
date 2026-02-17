package response

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	Get(ctx context.Context, id uuid.UUID) (FormResponse, error)
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	Create(ctx context.Context, arg CreateParams) (FormResponse, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]FormResponse, error)
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("response/service"),
	}
}

// Create creates an empty response (draft) for a given form and user
// Returns an error if the user already has a response for the form
func (s Service) Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	// Check if user already has a response for this form
	exists, err := s.queries.Exists(traceCtx, ExistsParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check if response exists")
		span.RecordError(err)
		return FormResponse{}, err
	}

	if exists {
		err = fmt.Errorf("user already has a response for this form")
		logger.Error("Failed to create empty response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return FormResponse{}, internal.ErrResponseAlreadyExists
	}

	// Create empty response
	newResponse, err := s.queries.Create(traceCtx, CreateParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create empty response")
		span.RecordError(err)
		return FormResponse{}, err
	}

	return newResponse, nil
}

// ListByFormID retrieves all responses for a given form
func (s Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	responses, err := s.queries.ListByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "form_id", formID.String(), logger, "list responses by form id")
		span.RecordError(err)
		return []FormResponse{}, err
	}

	return responses, nil
}

// ListBySubmittedBy retrieves all responses submitted by a given user
func (s Service) ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error) {
	ctx, span := s.tracer.Start(ctx, "ListBySubmittedBy")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	responses, err := s.queries.ListBySubmittedBy(ctx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list responses by submitted by")
		span.RecordError(err)
		return nil, err
	}

	return responses, nil
}

func (s Service) Get(ctx context.Context, id uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	response, err := s.queries.Get(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, err
	}

	return response, nil
}

func (s Service) GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetFormIDByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get form id by response id")
		span.RecordError(err)
		return uuid.Nil, nil
	}

	return formID, nil
}

// Delete deletes a response by id
func (s Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "delete response")
		span.RecordError(err)
		return err
	}

	return nil
}
