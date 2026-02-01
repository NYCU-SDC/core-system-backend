package inbox

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"fmt"
	"net/http"
	"time"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	pagutil "github.com/NYCU-SDC/summer/pkg/pagination"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Store interface {
	List(ctx context.Context, userID uuid.UUID, filter *FilterRequest, page int, size int) ([]ListRow, error)
	Count(ctx context.Context, userID uuid.UUID, filter *FilterRequest) (int64, error)
	GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (GetByIDRow, error)
	UpdateByID(ctx context.Context, id uuid.UUID, userID uuid.UUID, arg UserInboxMessageFilter) (UpdateByIDRow, error)
}

type UserInboxMessageFilter struct {
	IsRead     bool `json:"isRead"`
	IsStarred  bool `json:"isStarred"`
	IsArchived bool `json:"isArchived"`
}

type FormMessageResponse struct {
	ID             string      `json:"id"`
	PostedBy       string      `json:"postedBy"`
	Title          string      `json:"title"`
	Org            string      `json:"org"`
	Unit           string      `json:"unit"`
	Type           ContentType `json:"type"`
	PreviewMessage string      `json:"previewMessage"`
	ContentID      string      `json:"contentId"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
}

type Response struct {
	ID      string              `json:"id"`
	Message FormMessageResponse `json:"message"`
	UserInboxMessageFilter
}

type ResponseDetail struct {
	ID      string              `json:"id"`
	Message FormMessageResponse `json:"message"`
	Content any                 `json:"content"`
	UserInboxMessageFilter
}

type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store     Store
	formStore form.Store
	unitStore unit.Store
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	formStore form.Store,
	unitStore unit.Store,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		tracer:        otel.Tracer("inbox/handler"),
		store:         store,
		formStore:     formStore,
		unitStore:     unitStore,
	}
}

func (h *Handler) mapToResponse(ctx context.Context, message ListRow) (Response, error) {
	traceCtx, span := h.tracer.Start(ctx, "mapToResponse")
	defer span.End()

	previewMessage := h.extractStringField(traceCtx, message.PreviewMessage)
	title := h.extractStringField(traceCtx, message.Title)
	orgName := h.extractStringField(traceCtx, message.OrgName)
	unitName := h.extractStringField(traceCtx, message.UnitName)

	return Response{
		ID: message.ID.String(),
		Message: FormMessageResponse{
			ID:             message.MessageID.String(),
			PostedBy:       message.PostedBy.String(),
			Title:          title,
			Org:            orgName,
			Unit:           unitName,
			Type:           message.Type,
			PreviewMessage: previewMessage,
			ContentID:      message.ContentID.String(),
			CreatedAt:      message.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      message.UpdatedAt.Time.Format(time.RFC3339),
		},
		UserInboxMessageFilter: UserInboxMessageFilter{
			IsRead:     message.IsRead,
			IsStarred:  message.IsStarred,
			IsArchived: message.IsArchived,
		},
	}, nil
}

// extractStringField extracts a string field from the database result
func (h *Handler) extractStringField(ctx context.Context, field interface{}) string {
	traceCtx, span := h.tracer.Start(ctx, "extractStringField", trace.WithAttributes(
		attribute.String("field", fmt.Sprintf("%v", field)),
		attribute.String("field_type", fmt.Sprintf("%T", field)),
	))
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	if field != nil {
		fieldStr, ok := field.(string)
		if ok {
			return fieldStr
		} else {
			logutil.WithContext(traceCtx, logger).Warn("field type mismatch",
				zap.String("field_type", fmt.Sprintf("%T", field)),
				zap.String("field", fmt.Sprintf("%v", field)),
			)
			return ""
		}
	}
	return ""
}

func (h *Handler) GetMessageContent(ctx context.Context, contentType ContentType, contentID uuid.UUID) (any, error) {
	traceCtx, span := h.tracer.Start(ctx, "GetMessageContent")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	switch contentType {
	case ContentTypeForm:
		currentForm, err := h.formStore.GetByID(traceCtx, contentID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "get form by id")
			span.RecordError(err)
			return form.Response{}, err
		}
		response := form.ToResponse(form.Form{
			ID:             currentForm.ID,
			Title:          currentForm.Title,
			Description:    currentForm.Description,
			PreviewMessage: currentForm.PreviewMessage,
			Status:         currentForm.Status,
			UnitID:         currentForm.UnitID,
			LastEditor:     currentForm.LastEditor,
			Deadline:       currentForm.Deadline,
			CreatedAt:      currentForm.CreatedAt,
			UpdatedAt:      currentForm.UpdatedAt,
		}, currentForm.UnitName.String, currentForm.OrgName.String, user.User{
			ID:        currentForm.LastEditor,
			Name:      currentForm.LastEditorName,
			Username:  currentForm.LastEditorUsername,
			AvatarUrl: currentForm.LastEditorAvatarUrl,
		}, user.ConvertEmailsToSlice(currentForm.LastEditorEmail))
		return response, nil
	case ContentTypeText:
		return nil, nil
	}

	return nil, fmt.Errorf("content type %s not supported", contentType)
}

func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	factory := pagutil.NewFactory[Response](200, []string{"CreatedAt"})
	request, err := factory.GetRequest(r)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse filter parameters
	filter, err := ParseFilterRequest(r)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Get total count for pagination
	total, err := h.store.Count(traceCtx, currentUser.ID, filter)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	messages, err := h.store.List(traceCtx, currentUser.ID, filter, request.Page, request.Size)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	mappedMessage := make([]Response, len(messages))
	for i, message := range messages {
		mappedMessage[i], err = h.mapToResponse(traceCtx, message)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
	}

	response := factory.NewResponse(mappedMessage, int(total), request.Page, request.Size)

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) GetHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	pathID := r.PathValue("id")
	id, err := internal.ParseUUID(pathID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	message, err := h.store.GetByID(traceCtx, id, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	contentID, err := internal.ParseUUID(message.ContentID.String())
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	messageContent, err := h.GetMessageContent(traceCtx, message.Type, contentID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	previewMessage := h.extractStringField(traceCtx, message.PreviewMessage)
	title := h.extractStringField(traceCtx, message.Title)
	orgName := h.extractStringField(traceCtx, message.OrgName)
	unitName := h.extractStringField(traceCtx, message.UnitName)

	response := ResponseDetail{
		ID: message.ID.String(),
		Message: FormMessageResponse{
			ID:             message.MessageID.String(),
			PostedBy:       message.PostedBy.String(),
			Title:          title,
			Org:            orgName,
			Unit:           unitName,
			Type:           message.Type,
			PreviewMessage: previewMessage,
			ContentID:      message.ContentID.String(),
			CreatedAt:      message.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      message.UpdatedAt.Time.Format(time.RFC3339),
		},
		Content: messageContent,
		UserInboxMessageFilter: UserInboxMessageFilter{
			IsRead:     message.IsRead,
			IsStarred:  message.IsStarred,
			IsArchived: message.IsArchived,
		},
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	pathID := r.PathValue("id")
	id, err := internal.ParseUUID(pathID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req UserInboxMessageFilter
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

	message, err := h.store.UpdateByID(traceCtx, id, currentUser.ID, UserInboxMessageFilter{
		IsRead:     req.IsRead,
		IsStarred:  req.IsStarred,
		IsArchived: req.IsArchived,
	})
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	previewMessage := h.extractStringField(traceCtx, message.PreviewMessage)
	title := h.extractStringField(traceCtx, message.Title)
	orgName := h.extractStringField(traceCtx, message.OrgName)
	unitName := h.extractStringField(traceCtx, message.UnitName)

	response := Response{
		ID: message.ID.String(),
		Message: FormMessageResponse{
			ID:             message.MessageID.String(),
			PostedBy:       message.PostedBy.String(),
			Title:          title,
			Org:            orgName,
			Unit:           unitName,
			Type:           message.Type,
			PreviewMessage: previewMessage,
			ContentID:      message.ContentID.String(),
			CreatedAt:      message.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      message.UpdatedAt.Time.Format(time.RFC3339),
		},
		UserInboxMessageFilter: UserInboxMessageFilter{
			IsRead:     message.IsRead,
			IsStarred:  message.IsStarred,
			IsArchived: message.IsArchived,
		},
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}
