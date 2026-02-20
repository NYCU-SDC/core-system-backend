package form

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/file"
	"NYCU-SDC/core-system-backend/internal/form/font"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type DressingRequest struct {
	Color        string `json:"color" validate:"omitempty,hexcolor"`
	HeaderFont   string `json:"headerFont" validate:"omitempty,font"`
	QuestionFont string `json:"questionFont" validate:"omitempty,font"`
	TextFont     string `json:"textFont" validate:"omitempty,font"`
}

type Request struct {
	Title                  string           `json:"title" validate:"required"`
	Description            string           `json:"description"`
	PreviewMessage         string           `json:"previewMessage"`
	Deadline               *time.Time       `json:"deadline"`
	PublishTime            *time.Time       `json:"publishTime"`
	MessageAfterSubmission string           `json:"messageAfterSubmission"`
	GoogleSheetUrl         string           `json:"googleSheetUrl"`
	Visibility             string           `json:"visibility" validate:"required,oneof=PUBLIC PRIVATE"`
	CoverImageUrl          string           `json:"coverImageUrl"`
	Dressing               *DressingRequest `json:"dressing"`
}

type PatchRequest struct {
	Title                  *string          `json:"title" validate:"omitempty"`
	Description            *string          `json:"description" validate:"omitempty"`
	PreviewMessage         *string          `json:"previewMessage"`
	Deadline               *time.Time       `json:"deadline"`
	PublishTime            *time.Time       `json:"publishTime"`
	MessageAfterSubmission *string          `json:"messageAfterSubmission" validate:"omitempty"`
	GoogleSheetUrl         *string          `json:"googleSheetUrl"`
	Visibility             *string          `json:"visibility" validate:"omitempty,oneof=PUBLIC PRIVATE"`
	CoverImageUrl          *string          `json:"coverImageUrl"`
	Dressing               *DressingRequest `json:"dressing"`
}

type Response struct {
	ID                     string               `json:"id"`
	Title                  string               `json:"title"`
	Description            string               `json:"description"`
	PreviewMessage         string               `json:"previewMessage"`
	Status                 string               `json:"status"`
	UnitID                 string               `json:"unitId"`
	LastEditor             user.ProfileResponse `json:"lastEditor"`
	Deadline               *time.Time           `json:"deadline"`
	CreatedAt              time.Time            `json:"createdAt"`
	UpdatedAt              time.Time            `json:"updatedAt"`
	PublishTime            *time.Time           `json:"publishTime"`
	MessageAfterSubmission string               `json:"messageAfterSubmission"`
	GoogleSheetUrl         string               `json:"googleSheetUrl"`
	Visibility             string               `json:"visibility"`
	CoverImageUrl          string               `json:"coverImageUrl"`
	Dressing               DressingRequest      `json:"dressing"`
}

type CoverUploadResponse struct {
	ImageURL string `json:"imageUrl"`
}

type SectionRequest struct {
	Title       string  `json:"title" validate:"required"`
	Description *string `json:"description"`
}

type SectionResponse struct {
	ID          string `json:"id"`
	FormID      string `json:"formId"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// statusToUppercase converts database status format (lowercase) to API format (uppercase).
func statusToUppercase(s Status) string {
	switch s {
	case StatusDraft:
		return "DRAFT"
	case StatusPublished:
		return "PUBLISHED"
	case StatusArchived:
		return "ARCHIVED"
	default:
		return string(s)
	}
}

// VisibilityToUppercase converts database visibility format (lowercase) to API format (uppercase).
func VisibilityToUppercase(v Visibility) string {
	switch v {
	case VisibilityPublic:
		return "PUBLIC"
	case VisibilityPrivate:
		return "PRIVATE"
	default:
		return string(v)
	}
}

// ToResponse converts a Form storage model into an API Response.
// Ensures deadline, publishTime is null when empty/invalid.
func ToResponse(form Form, unitName string, orgName string, editor user.User, emails []string) Response {
	var deadline *time.Time

	if form.Deadline.Valid {
		deadline = &form.Deadline.Time
	} else {
		deadline = nil
	}

	var publishTime *time.Time
	if form.PublishTime.Valid {
		publishTime = &form.PublishTime.Time
	} else {
		publishTime = nil
	}

	return Response{
		ID:             form.ID.String(),
		Title:          form.Title,
		Description:    form.Description.String,
		PreviewMessage: form.PreviewMessage.String,
		Status:         statusToUppercase(form.Status),
		UnitID:         form.UnitID.String(),
		LastEditor: user.ProfileResponse{
			ID:        editor.ID,
			Name:      editor.Name.String,
			Username:  editor.Username.String,
			Emails:    emails,
			AvatarURL: editor.AvatarUrl.String,
		},
		Deadline:               deadline,
		CreatedAt:              form.CreatedAt.Time,
		UpdatedAt:              form.UpdatedAt.Time,
		MessageAfterSubmission: form.MessageAfterSubmission,
		Visibility:             VisibilityToUppercase(form.Visibility),
		GoogleSheetUrl:         form.GoogleSheetUrl.String,
		PublishTime:            publishTime,
		CoverImageUrl:          form.CoverImageUrl.String,
		Dressing: DressingRequest{
			Color:        form.DressingColor.String,
			HeaderFont:   form.DressingHeaderFont.String,
			QuestionFont: form.DressingQuestionFont.String,
			TextFont:     form.DressingTextFont.String,
		},
	}
}

type Store interface {
	Create(ctx context.Context, request Request, unitID uuid.UUID, userID uuid.UUID) (CreateRow, error)
	Patch(ctx context.Context, id uuid.UUID, request PatchRequest, userID uuid.UUID) (PatchRow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (GetByIDRow, error)
	List(ctx context.Context) ([]ListRow, error)
	ListByUnit(ctx context.Context, unitID uuid.UUID) ([]ListByUnitRow, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status, userID uuid.UUID) (Form, error)
	UploadCoverImage(ctx context.Context, id uuid.UUID, data []byte, coverImageURL string) error
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
}

type tenantStore interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type questionStore interface {
	UpdateSection(ctx context.Context, sectionID uuid.UUID, formID uuid.UUID, title string, description string) (question.Section, error)
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store         Store
	tenantStore   tenantStore
	questionStore questionStore
	fileService   fileService
}

type fileService interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	GetByID(ctx context.Context, id uuid.UUID) (file.File, error)
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	tenantStore tenantStore,
	questionStore questionStore,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("form/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tenantStore:   tenantStore,
		questionStore: questionStore,
		fileService:   fileService,
	}
}

func (h *Handler) PatchHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "PatchHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var req PatchRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	currentForm, err := h.store.Patch(traceCtx, id, req, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(Form{
		ID:                     currentForm.ID,
		Title:                  currentForm.Title,
		Description:            currentForm.Description,
		PreviewMessage:         currentForm.PreviewMessage,
		Status:                 currentForm.Status,
		UnitID:                 currentForm.UnitID,
		LastEditor:             currentForm.LastEditor,
		Deadline:               currentForm.Deadline,
		CreatedAt:              currentForm.CreatedAt,
		UpdatedAt:              currentForm.UpdatedAt,
		MessageAfterSubmission: currentForm.MessageAfterSubmission,
		Visibility:             currentForm.Visibility,
		GoogleSheetUrl:         currentForm.GoogleSheetUrl,
		PublishTime:            currentForm.PublishTime,
		CoverImageUrl:          currentForm.CoverImageUrl,
		DressingColor:          currentForm.DressingColor,
		DressingHeaderFont:     currentForm.DressingHeaderFont,
		DressingQuestionFont:   currentForm.DressingQuestionFont,
		DressingTextFont:       currentForm.DressingTextFont,
	},
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		user.User{
			ID:        currentForm.LastEditor,
			Name:      currentForm.LastEditorName,
			Username:  currentForm.LastEditorUsername,
			AvatarUrl: currentForm.LastEditorAvatarUrl,
		},
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail))
	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) GetHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentForm, err := h.store.GetByID(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(Form{
		ID:                     currentForm.ID,
		Title:                  currentForm.Title,
		Description:            currentForm.Description,
		PreviewMessage:         currentForm.PreviewMessage,
		Status:                 currentForm.Status,
		UnitID:                 currentForm.UnitID,
		LastEditor:             currentForm.LastEditor,
		Deadline:               currentForm.Deadline,
		CreatedAt:              currentForm.CreatedAt,
		UpdatedAt:              currentForm.UpdatedAt,
		MessageAfterSubmission: currentForm.MessageAfterSubmission,
		Visibility:             currentForm.Visibility,
		GoogleSheetUrl:         currentForm.GoogleSheetUrl,
		PublishTime:            currentForm.PublishTime,
		CoverImageUrl:          currentForm.CoverImageUrl,
		DressingColor:          currentForm.DressingColor,
		DressingHeaderFont:     currentForm.DressingHeaderFont,
		DressingQuestionFont:   currentForm.DressingQuestionFont,
		DressingTextFont:       currentForm.DressingTextFont,
	},
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		user.User{
			ID:        currentForm.LastEditor,
			Name:      currentForm.LastEditorName,
			Username:  currentForm.LastEditorUsername,
			AvatarUrl: currentForm.LastEditorAvatarUrl,
		},
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail))
	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	forms, err := h.store.List(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, 0, len(forms))
	for _, form := range forms {
		responses = append(responses, ToResponse(Form{
			ID:                     form.ID,
			Title:                  form.Title,
			Description:            form.Description,
			PreviewMessage:         form.PreviewMessage,
			Status:                 form.Status,
			MessageAfterSubmission: form.MessageAfterSubmission,
			Visibility:             form.Visibility,
			GoogleSheetUrl:         form.GoogleSheetUrl,
			PublishTime:            form.PublishTime,
			CoverImageUrl:          form.CoverImageUrl,
			DressingColor:          form.DressingColor,
			DressingHeaderFont:     form.DressingHeaderFont,
			DressingQuestionFont:   form.DressingQuestionFont,
			DressingTextFont:       form.DressingTextFont,
		},
			form.UnitName.String,
			form.OrgName.String,
			user.User{
				ID:        form.LastEditor,
				Name:      form.LastEditorName,
				Username:  form.LastEditorUsername,
				AvatarUrl: form.LastEditorAvatarUrl,
			},
			user.ConvertEmailsToSlice(form.LastEditorEmail)))
	}
	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

func (h *Handler) CreateUnderOrgHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateUnderOrgHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), logger)
		return
	}

	_, orgID, err := h.tenantStore.GetSlugStatus(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), logger)
		return
	}

	newForm, err := h.store.Create(traceCtx, req, orgID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(Form{
		ID:                     newForm.ID,
		Title:                  newForm.Title,
		Description:            newForm.Description,
		PreviewMessage:         newForm.PreviewMessage,
		Status:                 newForm.Status,
		UnitID:                 newForm.UnitID,
		LastEditor:             newForm.LastEditor,
		Deadline:               newForm.Deadline,
		CreatedAt:              newForm.CreatedAt,
		UpdatedAt:              newForm.UpdatedAt,
		MessageAfterSubmission: newForm.MessageAfterSubmission,
		Visibility:             newForm.Visibility,
		GoogleSheetUrl:         newForm.GoogleSheetUrl,
		PublishTime:            newForm.PublishTime,
		CoverImageUrl:          newForm.CoverImageUrl,
		DressingColor:          newForm.DressingColor,
		DressingHeaderFont:     newForm.DressingHeaderFont,
		DressingQuestionFont:   newForm.DressingQuestionFont,
		DressingTextFont:       newForm.DressingTextFont,
	},
		newForm.UnitName.String,
		newForm.OrgName.String,
		user.User{
			ID:        newForm.LastEditor,
			Name:      newForm.LastEditorName,
			Username:  newForm.LastEditorUsername,
			AvatarUrl: newForm.LastEditorAvatarUrl,
		},
		user.ConvertEmailsToSlice(newForm.LastEditorEmail))
	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
}

func (h *Handler) ListByOrgHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListByOrgHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), logger)
		return
	}

	_, orgID, err := h.tenantStore.GetSlugStatus(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), logger)
		return
	}

	forms, err := h.store.ListByUnit(traceCtx, orgID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, len(forms))
	for i, currentForm := range forms {
		responses[i] = ToResponse(Form{
			ID:                     currentForm.ID,
			Title:                  currentForm.Title,
			Description:            currentForm.Description,
			PreviewMessage:         currentForm.PreviewMessage,
			Status:                 currentForm.Status,
			UnitID:                 currentForm.UnitID,
			LastEditor:             currentForm.LastEditor,
			Deadline:               currentForm.Deadline,
			CreatedAt:              currentForm.CreatedAt,
			UpdatedAt:              currentForm.UpdatedAt,
			MessageAfterSubmission: currentForm.MessageAfterSubmission,
			Visibility:             currentForm.Visibility,
			GoogleSheetUrl:         currentForm.GoogleSheetUrl,
			PublishTime:            currentForm.PublishTime,
			CoverImageUrl:          currentForm.CoverImageUrl,
			DressingColor:          currentForm.DressingColor,
			DressingHeaderFont:     currentForm.DressingHeaderFont,
			DressingQuestionFont:   currentForm.DressingQuestionFont,
			DressingTextFont:       currentForm.DressingTextFont,
		}, currentForm.UnitName.String, currentForm.OrgName.String, user.User{
			ID:        currentForm.LastEditor,
			Name:      currentForm.LastEditorName,
			Username:  currentForm.LastEditorUsername,
			AvatarUrl: currentForm.LastEditorAvatarUrl,
		}, user.ConvertEmailsToSlice(currentForm.LastEditorEmail))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

func (h *Handler) UploadCoverImageHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UploadCoverImageHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	const maxBytes int64 = 2 << 20 // 2MB

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidMultipart, logger)
		return
	}

	fileData, header, err := r.FormFile("coverImage")
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInvalidMultipart, logger)
		return
	}
	defer func() {
		if err := fileData.Close(); err != nil {
			logutil.WithContext(traceCtx, logger).Warn(
				"failed to close cover image file",
				zap.String("form_id", formID.String()),
				zap.Error(err),
			)
		}
	}()

	// Save to file service with WebP validation (system upload, no user attribution)
	savedFile, err := h.fileService.SaveFile(
		traceCtx,
		fileData,
		header.Filename,
		"image/webp",
		nil, // system upload
		file.WithWebP(),
		file.WithMaxSize(maxBytes),
	)
	if err != nil {
		logger.Error("Failed to save cover image", zap.Error(err))
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		span.RecordError(err)
		return
	}

	// Update form's cover_image_url
	coverImageURL := fmt.Sprintf("/api/forms/%s/cover", formID.String())
	if err := h.store.UploadCoverImage(traceCtx, formID, savedFile.Data, coverImageURL); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, CoverUploadResponse{ImageURL: coverImageURL})
}

func (h *Handler) GetCoverImageHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetCoverImageHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	imageData, err := h.store.GetCoverImage(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	w.Header().Set("Content-Type", "image/webp")
	_, err = w.Write(imageData)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
}

func (h *Handler) ArchiveHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ArchiveHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("formId")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	_, err = h.store.SetStatus(traceCtx, id, StatusArchived, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentForm, err := h.store.GetByID(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(Form{
		ID:                     currentForm.ID,
		Title:                  currentForm.Title,
		Description:            currentForm.Description,
		PreviewMessage:         currentForm.PreviewMessage,
		Status:                 currentForm.Status,
		UnitID:                 currentForm.UnitID,
		LastEditor:             currentForm.LastEditor,
		Deadline:               currentForm.Deadline,
		CreatedAt:              currentForm.CreatedAt,
		UpdatedAt:              currentForm.UpdatedAt,
		MessageAfterSubmission: currentForm.MessageAfterSubmission,
		Visibility:             currentForm.Visibility,
		GoogleSheetUrl:         currentForm.GoogleSheetUrl,
		PublishTime:            currentForm.PublishTime,
		CoverImageUrl:          currentForm.CoverImageUrl,
		DressingColor:          currentForm.DressingColor,
		DressingHeaderFont:     currentForm.DressingHeaderFont,
		DressingQuestionFont:   currentForm.DressingQuestionFont,
		DressingTextFont:       currentForm.DressingTextFont,
	},
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		user.User{
			ID:        currentForm.LastEditor,
			Name:      currentForm.LastEditorName,
			Username:  currentForm.LastEditorUsername,
			AvatarUrl: currentForm.LastEditorAvatarUrl,
		},
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail))

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) GetFontsHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetFontsHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	fonts, err := font.List()
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, fonts)
}

func (h *Handler) UpdateSectionHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateSectionHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := handlerutil.ParseUUID(formIDStr)
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

	var req SectionRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	section, err := h.questionStore.UpdateSection(traceCtx, sectionID, formID, req.Title, description)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := SectionResponse{
		ID:          section.ID.String(),
		FormID:      section.FormID.String(),
		Title:       section.Title.String,
		Description: section.Description.String,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}
