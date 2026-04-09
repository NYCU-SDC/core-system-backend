package highlight

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Querier interface {
	GetByFormID(ctx context.Context, formID uuid.UUID) (FormHighlight, error)
	UpsertByFormID(ctx context.Context, arg UpsertByFormIDParams) (FormHighlight, error)
	DeleteByFormID(ctx context.Context, formID uuid.UUID) error
	GetQuestionByFormIDAndQuestionID(ctx context.Context, arg GetQuestionByFormIDAndQuestionIDParams) (GetQuestionByFormIDAndQuestionIDRow, error)
	ListAnswerValuesByQuestionID(ctx context.Context, questionID uuid.UUID) ([][]byte, error)
}

type FormStore interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type Request struct {
	QuestionID   *uuid.UUID
	DisplayTitle *string
}

type ChoiceStat struct {
	ChoiceID uuid.UUID `json:"choiceId"`
	Name     string    `json:"name"`
	Count    int32     `json:"count"`
}

type Response struct {
	QuestionID    *uuid.UUID   `json:"questionId"`
	QuestionTitle *string      `json:"questionTitle"`
	DisplayTitle  *string      `json:"displayTitle"`
	Choices       []ChoiceStat `json:"choices"`
}

type Service struct {
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer

	formStore FormStore
}

func NewService(logger *zap.Logger, db DBTX, formStore FormStore) *Service {
	return &Service{
		logger:    logger,
		queries:   New(db),
		tracer:    otel.Tracer("highlight/service"),
		formStore: formStore,
	}
}

func (s *Service) Get(ctx context.Context, formID uuid.UUID) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.formStore.Exists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return Response{}, err
	}
	if !exists {
		return emptyResponse(), internal.ErrFormNotFound
	}

	highlightRow, err := s.queries.GetByFormID(traceCtx, formID)
	if errors.Is(err, pgx.ErrNoRows) {
		return emptyResponse(), nil
	}
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get form highlight")
		span.RecordError(err)
		return Response{}, err
	}

	return s.buildResponse(traceCtx, formID, highlightRow.QuestionID, highlightRow.DisplayTitle)
}

func (s *Service) Set(ctx context.Context, formID uuid.UUID, req Request) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "Set")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exists, err := s.formStore.Exists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return Response{}, err
	}
	if !exists {
		return emptyResponse(), internal.ErrFormNotFound
	}

	if req.QuestionID == nil {
		if err := s.queries.DeleteByFormID(traceCtx, formID); err != nil {
			err = databaseutil.WrapDBError(err, logger, "delete form highlight")
			span.RecordError(err)
			return Response{}, err
		}
		return emptyResponse(), nil
	}

	questionRow, err := s.queries.GetQuestionByFormIDAndQuestionID(traceCtx, GetQuestionByFormIDAndQuestionIDParams{
		FormID: formID,
		ID:     *req.QuestionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Response{}, ErrQuestionNotInForm{FormID: formID.String(), QuestionID: req.QuestionID.String()}
	}
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get highlight question by form and question id")
		span.RecordError(err)
		return Response{}, err
	}

	if !isSupportedQuestionType(question.QuestionType(questionRow.Type)) {
		return Response{}, ErrUnsupportedHighlightQuestionType{
			QuestionID:   questionRow.ID.String(),
			QuestionType: string(questionRow.Type),
		}
	}

	displayTitle := normalizeDisplayTitle(req.DisplayTitle)
	highlightRow, err := s.queries.UpsertByFormID(traceCtx, UpsertByFormIDParams{
		FormID:       formID,
		QuestionID:   *req.QuestionID,
		DisplayTitle: displayTitle,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "upsert form highlight")
		span.RecordError(err)
		return Response{}, err
	}

	return s.buildResponse(traceCtx, formID, highlightRow.QuestionID, highlightRow.DisplayTitle)
}

func (s *Service) buildResponse(ctx context.Context, formID, questionID uuid.UUID, storedDisplayTitle pgtype.Text) (Response, error) {
	traceCtx, span := s.tracer.Start(ctx, "buildResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	questionRow, err := s.queries.GetQuestionByFormIDAndQuestionID(traceCtx, GetQuestionByFormIDAndQuestionIDParams{
		FormID: formID,
		ID:     questionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return emptyResponse(), nil
	}
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get highlighted question")
		span.RecordError(err)
		return Response{}, err
	}

	choices, err := question.ExtractChoices(questionRow.Metadata)
	if err != nil {
		span.RecordError(err)
		return Response{}, err
	}

	answerValues, err := s.queries.ListAnswerValuesByQuestionID(traceCtx, questionID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list highlight answer values")
		span.RecordError(err)
		return Response{}, err
	}

	choiceStats, err := countChoices(question.QuestionType(questionRow.Type), choices, answerValues)
	if err != nil {
		span.RecordError(err)
		return Response{}, err
	}

	questionTitle := questionRow.Title.String
	displayTitle := questionTitle
	if storedDisplayTitle.Valid && strings.TrimSpace(storedDisplayTitle.String) != "" {
		displayTitle = storedDisplayTitle.String
	}

	return Response{
		QuestionID:    &questionID,
		QuestionTitle: stringPtr(questionTitle),
		DisplayTitle:  stringPtr(displayTitle),
		Choices:       choiceStats,
	}, nil
}

func emptyResponse() Response {
	return Response{Choices: []ChoiceStat{}}
}

func normalizeDisplayTitle(displayTitle *string) pgtype.Text {
	if displayTitle == nil {
		return pgtype.Text{}
	}
	trimmed := strings.TrimSpace(*displayTitle)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func isSupportedQuestionType(questionType question.QuestionType) bool {
	switch questionType {
	case question.QuestionTypeSingleChoice,
		question.QuestionTypeMultipleChoice,
		question.QuestionTypeDropdown,
		question.QuestionTypeDetailedMultipleChoice:
		return true
	default:
		return false
	}
}

func countChoices(questionType question.QuestionType, choices []question.Choice, answerValues [][]byte) ([]ChoiceStat, error) {
	counts := make(map[uuid.UUID]int32, len(choices))
	for _, choice := range choices {
		counts[choice.ID] = 0
	}

	for _, rawValue := range answerValues {
		switch questionType {
		case question.QuestionTypeSingleChoice, question.QuestionTypeDropdown:
			var answer shared.SingleChoiceAnswer
			if err := json.Unmarshal(rawValue, &answer); err != nil {
				return nil, fmt.Errorf("decode single choice highlight answer: %w", err)
			}
			counts[answer.ChoiceID]++
		case question.QuestionTypeMultipleChoice:
			var answer shared.MultipleChoiceAnswer
			if err := json.Unmarshal(rawValue, &answer); err != nil {
				return nil, fmt.Errorf("decode multiple choice highlight answer: %w", err)
			}
			for _, choice := range answer.Choices {
				counts[choice.ChoiceID]++
			}
		case question.QuestionTypeDetailedMultipleChoice:
			var answer shared.DetailedMultipleChoiceAnswer
			if err := json.Unmarshal(rawValue, &answer); err != nil {
				return nil, fmt.Errorf("decode detailed multiple choice highlight answer: %w", err)
			}
			for _, choice := range answer.Choices {
				counts[choice.ChoiceID]++
			}
		default:
			return nil, ErrUnsupportedHighlightQuestionType{QuestionType: string(questionType)}
		}
	}

	stats := make([]ChoiceStat, 0, len(choices))
	for _, choice := range choices {
		stats = append(stats, ChoiceStat{
			ChoiceID: choice.ID,
			Name:     choice.Name,
			Count:    counts[choice.ID],
		})
	}

	return stats, nil
}

func stringPtr(value string) *string {
	return &value
}
