package question

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"net/http"
	"strings"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Request struct {
	Required     *bool            `json:"required" validate:"required"`
	Type         string           `json:"type" validate:"required,oneof=SHORT_TEXT LONG_TEXT SINGLE_CHOICE MULTIPLE_CHOICE DATE DROPDOWN DETAILED_MULTIPLE_CHOICE UPLOAD_FILE LINEAR_SCALE RATING RANKING OAUTH_CONNECT HYPERLINK"`
	Title        string           `json:"title" validate:"required"`
	Description  string           `json:"description"`
	Order        int32            `json:"order" validate:"required,min=1"`
	Choices      []ChoiceOption   `json:"choices,omitempty" validate:"omitempty,required_if=Type SINGLE_CHOICE,required_if=Type MULTIPLE_CHOICE,required_if=Type DETAILED_MULTIPLE_CHOICE,required_if=Type DROPDOWN,required_if=Type RANKING,dive"`
	Scale        ScaleOption      `json:"scale,omitempty" validate:"omitempty,required_if=Type LINEAR_SCALE,required_if=Type RATING"`
	UploadFile   UploadFileOption `json:"uploadFile,omitempty" validate:"omitempty,required_if=Type UPLOAD_FILE"`
	Date         DateOption       `json:"date,omitempty"`
	OauthConnect string           `json:"oauthConnect,omitempty" validate:"required_if=Type OAUTH_CONNECT"`
	SourceID     uuid.UUID        `json:"sourceId,omitempty"`
}

type Response struct {
	ID           uuid.UUID         `json:"id"`
	SectionID    uuid.UUID         `json:"sectionId"`
	Required     bool              `json:"required"`
	Type         string            `json:"type"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Choices      []Choice          `json:"choices,omitempty"`
	Scale        *ScaleOption      `json:"scale,omitempty"`
	UploadFile   *UploadFileOption `json:"uploadFile,omitempty"`
	Date         *DateOption       `json:"date,omitempty"`
	OauthConnect string            `json:"oauthConnect,omitempty"`
	SourceID     string            `json:"sourceId,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type SectionPayload struct {
	ID          uuid.UUID `json:"id"`
	FormID      uuid.UUID `json:"formId"`
	Title       string    `json:"title"`
	Progress    string    `json:"progress"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type SectionResponse struct {
	Section   SectionPayload `json:"section"`
	Questions []Response     `json:"questions"`
}

func ToSection(section Section) SectionPayload {
	return SectionPayload{
		ID:          section.ID,
		FormID:      section.FormID,
		Title:       section.Title.String,
		Description: section.Description.String,
		CreatedAt:   section.CreatedAt.Time,
		UpdatedAt:   section.UpdatedAt.Time,
	}
}

func ToResponse(answerable Answerable) (Response, error) {
	q := answerable.Question()

	response := Response{
		ID:          q.ID,
		SectionID:   q.SectionID,
		Required:    q.Required,
		Type:        strings.ToUpper(string(q.Type)),
		Title:       q.Title.String,
		Description: q.Description.String,
		CreatedAt:   q.CreatedAt.Time,
		UpdatedAt:   q.UpdatedAt.Time,
	}
	if q.SourceID.Valid {
		response.SourceID = q.SourceID.String()
		return response, nil
	}

	switch q.Type {
	case QuestionTypeSingleChoice, QuestionTypeMultipleChoice, QuestionTypeDetailedMultipleChoice, QuestionTypeRanking, QuestionTypeDropdown:
		choices, err := ExtractChoices(q.Metadata)
		if err != nil {
			return response, ErrInvalidMetadata{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		response.Choices = choices
	case QuestionTypeLinearScale:
		scale, err := ExtractLinearScale(q.Metadata)
		if err != nil {
			return response, ErrInvalidMetadata{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		response.Scale = &ScaleOption{
			Icon:          scale.Icon,
			MinVal:        scale.MinVal,
			MaxVal:        scale.MaxVal,
			MinValueLabel: scale.MinValueLabel,
			MaxValueLabel: scale.MaxValueLabel,
		}
	case QuestionTypeRating:
		rating, err := ExtractRating(q.Metadata)
		if err != nil {
			return response, ErrInvalidMetadata{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		response.Scale = &ScaleOption{
			Icon:          rating.Icon,
			MinVal:        rating.MinVal,
			MaxVal:        rating.MaxVal,
			MinValueLabel: rating.MinValueLabel,
			MaxValueLabel: rating.MaxValueLabel,
		}
	case QuestionTypeUploadFile:
		uploadFile, err := ExtractUploadFile(q.Metadata)
		if err != nil {
			return response, ErrInvalidMetadata{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		fileTypes := make([]string, len(uploadFile.AllowedFileTypes))
		for i, ft := range uploadFile.AllowedFileTypes {
			fileTypes[i] = string(ft)
		}
		response.UploadFile = &UploadFileOption{
			AllowedFileTypes: fileTypes,
			MaxFileAmount:    uploadFile.MaxFileAmount,
			MaxFileSizeLimit: string(uploadFile.MaxFileSizeLimit),
		}
	case QuestionTypeOauthConnect:
		provider, err := ExtractOauthConnect(q.Metadata)
		if err != nil {
			return response, ErrInvalidMetadata{
				QuestionID: q.ID.String(),
				RawData:    q.Metadata,
				Message:    err.Error(),
			}
		}
		response.OauthConnect = string(provider)
	case QuestionTypeDate:
		// If metadata is nil, use default values
		if q.Metadata == nil {
			response.Date = &DateOption{
				HasYear:  true,
				HasMonth: true,
				HasDay:   true,
			}
		} else {
			dateMetadata, err := ExtractDateMetadata(q.Metadata)
			if err != nil {
				return response, ErrInvalidMetadata{
					QuestionID: q.ID.String(),
					RawData:    q.Metadata,
					Message:    err.Error(),
				}
			}
			response.Date = &DateOption{
				HasYear:  dateMetadata.HasYear,
				HasMonth: dateMetadata.HasMonth,
				HasDay:   dateMetadata.HasDay,
				MinDate:  dateMetadata.MinDate,
				MaxDate:  dateMetadata.MaxDate,
			}
		}
	}

	return response, nil
}

type Store interface {
	Create(ctx context.Context, input CreateParams) (Answerable, error)
	Update(ctx context.Context, input UpdateParams) (Answerable, error)
	UpdateOrder(ctx context.Context, input UpdateOrderParams) (Answerable, error)
	DeleteAndReorder(ctx context.Context, sectionID uuid.UUID, id uuid.UUID) error
	ListSectionsWithAnswersByFormID(ctx context.Context, formID uuid.UUID) ([]SectionWithAnswerableList, error)
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
		tracer:        otel.Tracer("question/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
	}
}

func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	sectionIDStr := r.PathValue("sectionId")
	sectionID, err := handlerutil.ParseUUID(sectionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	req.Type = strings.ToLower(req.Type)

	// Generate and validate metadata (returns nil if source_id provided)
	metadata, err := getGenerateMetadata(req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	request := CreateParams{
		SectionID:   sectionID,
		Required:    *req.Required,
		Type:        QuestionType(req.Type),
		Title:       pgtype.Text{String: req.Title, Valid: true},
		Description: pgtype.Text{String: req.Description, Valid: true},
		Order:       req.Order,
		Metadata:    metadata,
		SourceID:    pgtype.UUID{Bytes: req.SourceID, Valid: req.SourceID != uuid.Nil},
	}

	createdQuestion, err := h.store.Create(r.Context(), request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := ToResponse(createdQuestion)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
}

func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("questionId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	sectionIDStr := r.PathValue("sectionId")
	sectionID, err := handlerutil.ParseUUID(sectionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req Request
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	req.Type = strings.ToLower(req.Type)

	// Generate and validate metadata
	metadata, err := getGenerateMetadata(req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	request := UpdateParams{
		ID:          id,
		SectionID:   sectionID,
		Required:    *req.Required,
		Type:        QuestionType(req.Type),
		Title:       pgtype.Text{String: req.Title, Valid: true},
		Description: pgtype.Text{String: req.Description, Valid: true},
		Metadata:    metadata,
		SourceID:    pgtype.UUID{Bytes: req.SourceID, Valid: req.SourceID != uuid.Nil},
	}

	updatedQuestion, err := h.store.Update(traceCtx, request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	if updatedQuestion.Question().Order != req.Order {
		orderRequest := UpdateOrderParams{
			ID:        id,
			SectionID: sectionID,
			Order:     req.Order,
		}

		updatedQuestion, err = h.store.UpdateOrder(traceCtx, orderRequest)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
	}

	response, err := ToResponse(updatedQuestion)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	sectionIDStr := r.PathValue("sectionId")
	sectionID, err := handlerutil.ParseUUID(sectionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	idStr := r.PathValue("questionId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.DeleteAndReorder(traceCtx, sectionID, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	sectionWithQuestions, err := h.store.ListSectionsWithAnswersByFormID(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]SectionResponse, len(sectionWithQuestions))
	for i, s := range sectionWithQuestions {
		responses[i].Section = ToSection(sectionWithQuestions[i].Section)
		for _, q := range s.AnswerableList {
			response, err := ToResponse(q)
			if err != nil {
				h.problemWriter.WriteError(traceCtx, w, err, logger)
				return
			}
			responses[i].Questions = append(responses[i].Questions, response)
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

func getGenerateMetadata(req Request) ([]byte, error) {
	// If source_id is provided, don't generate metadata
	if req.SourceID != uuid.Nil {
		questionType := QuestionType(req.Type)
		switch questionType {
		case QuestionTypeSingleChoice, QuestionTypeMultipleChoice, QuestionTypeDropdown, QuestionTypeRanking:
			if len(req.Choices) > 0 {
				return nil, internal.ErrInvalidSourceIDWithChoices
			}
			return nil, nil
		default:
			return nil, internal.ErrInvalidSourceIDForType
		}
	}

	questionType := QuestionType(req.Type)
	switch questionType {
	case QuestionTypeShortText, QuestionTypeLongText, QuestionTypeHyperlink:
		return nil, nil
	case QuestionTypeDate:
		return GenerateDateMetadata(req.Date)
	case QuestionTypeSingleChoice, QuestionTypeMultipleChoice, QuestionTypeDetailedMultipleChoice, QuestionTypeDropdown, QuestionTypeRanking:
		return GenerateChoiceMetadata(req.Type, req.Choices)
	case QuestionTypeLinearScale:
		return GenerateLinearScaleMetadata(req.Scale)
	case QuestionTypeRating:
		return GenerateRatingMetadata(req.Scale)
	case QuestionTypeOauthConnect:
		return GenerateOauthConnectMetadata(req.OauthConnect)
	case QuestionTypeUploadFile:
		return GenerateUploadFileMetadata(req.UploadFile)
	default:
		return nil, ErrUnsupportedQuestionType{
			QuestionType: req.Type,
		}
	}
}
