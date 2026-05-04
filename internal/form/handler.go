package form

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/file"
	"NYCU-SDC/core-system-backend/internal/form/font"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/markdown"
	"NYCU-SDC/core-system-backend/internal/user"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

type DressingRequest struct {
	Color        string `json:"color" validate:"omitempty,hexcolor"`
	HeaderFont   string `json:"headerFont" validate:"omitempty,font"`
	QuestionFont string `json:"questionFont" validate:"omitempty,font"`
	TextFont     string `json:"textFont" validate:"omitempty,font"`
}

type Request struct {
	Title                  string           `json:"title" validate:"required"`
	Description            json.RawMessage  `json:"description"`
	PreviewMessage         string           `json:"previewMessage"`
	Deadline               *time.Time       `json:"deadline"`
	PublishTime            *time.Time       `json:"publishTime"`
	MessageAfterSubmission string           `json:"messageAfterSubmission"`
	GoogleSheetURL         string           `json:"googleSheetUrl"`
	Visibility             string           `json:"visibility" validate:"required,oneof=PUBLIC PRIVATE"`
	CoverImageURL          string           `json:"coverImageUrl"`
	Dressing               *DressingRequest `json:"dressing"`
}

type PatchRequest struct {
	Title                  *string            `json:"title" validate:"omitempty"`
	Description            OptionalRawMessage `json:"description"`
	PreviewMessage         *string            `json:"previewMessage"`
	Deadline               *time.Time         `json:"deadline"`
	PublishTime            *time.Time         `json:"publishTime"`
	MessageAfterSubmission *string            `json:"messageAfterSubmission" validate:"omitempty"`
	GoogleSheetURL         *string            `json:"googleSheetUrl"`
	Visibility             *string            `json:"visibility" validate:"omitempty,oneof=PUBLIC PRIVATE"`
	CoverImageURL          *string            `json:"coverImageUrl"`
	Dressing               *DressingRequest   `json:"dressing"`
}

type Response struct {
	ID                     string               `json:"id"`
	Title                  string               `json:"title"`
	Description            json.RawMessage      `json:"description"`
	DescriptionHTML        string               `json:"descriptionHtml,omitempty"`
	PreviewMessage         string               `json:"previewMessage"`
	Status                 string               `json:"status"`
	UnitID                 string               `json:"unitId"`
	Creator                user.ProfileResponse `json:"creator"`
	LastEditor             user.ProfileResponse `json:"lastEditor"`
	Deadline               *time.Time           `json:"deadline"`
	CreatedAt              time.Time            `json:"createdAt"`
	UpdatedAt              time.Time            `json:"updatedAt"`
	PublishTime            *time.Time           `json:"publishTime"`
	MessageAfterSubmission string               `json:"messageAfterSubmission"`
	GoogleSheetURL         string               `json:"googleSheetUrl"`
	Visibility             string               `json:"visibility"`
	CoverImageURL          string               `json:"coverImageUrl"`
	Dressing               DressingRequest      `json:"dressing"`
}

type CoverUploadResponse struct {
	ImageURL string `json:"imageUrl"`
}

type SectionRequest struct {
	Title       string             `json:"title" validate:"required"`
	Description OptionalRawMessage `json:"description"`
}

type SectionResponse struct {
	ID              string          `json:"id"`
	FormID          string          `json:"formId"`
	Title           string          `json:"title"`
	Description     json.RawMessage `json:"description,omitempty"`
	DescriptionHTML string          `json:"descriptionHtml,omitempty"`
}

// OptionalRawMessage allows PATCH payloads to distinguish:
// - field absent: Set=false (no change)
// - field present as null: Set=true, Value=nil (clear)
// - field present as JSON object/string/etc: Set=true, Value=<raw bytes>
type OptionalRawMessage struct {
	Set   bool
	Value json.RawMessage
}

func (o *OptionalRawMessage) UnmarshalJSON(b []byte) error {
	o.Set = true
	trimmed := bytes.TrimSpace(b)
	if bytes.Equal(trimmed, []byte("null")) {
		o.Value = nil
		return nil
	}

	o.Value = append(o.Value[:0], trimmed...)
	return nil
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
func ToResponse(
	form Form,
	unitName string,
	orgName string,
	creator user.User,
	creatorEmails []string,
	lastEditor user.User,
	lastEditorEmails []string,
) Response {
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

	desc := markdown.DefaultDescriptionJSON(form.DescriptionJson)
	return Response{
		ID:              form.ID.String(),
		Title:           form.Title,
		Description:     desc,
		DescriptionHTML: form.DescriptionHtml,
		PreviewMessage:  form.PreviewMessage.String,
		Status:          statusToUppercase(form.Status),
		UnitID:          form.UnitID.String(),
		Creator: user.ProfileResponse{
			ID:        creator.ID,
			Name:      creator.Name.String,
			Username:  creator.Username.String,
			Emails:    creatorEmails,
			AvatarURL: creator.AvatarUrl.String,
		},
		LastEditor: user.ProfileResponse{
			ID:        lastEditor.ID,
			Name:      lastEditor.Name.String,
			Username:  lastEditor.Username.String,
			Emails:    lastEditorEmails,
			AvatarURL: lastEditor.AvatarUrl.String,
		},
		Deadline:               deadline,
		CreatedAt:              form.CreatedAt.Time,
		UpdatedAt:              form.UpdatedAt.Time,
		MessageAfterSubmission: form.MessageAfterSubmission,
		Visibility:             VisibilityToUppercase(form.Visibility),
		GoogleSheetURL:         form.GoogleSheetUrl.String,
		PublishTime:            publishTime,
		CoverImageURL:          form.CoverImageUrl.String,
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
	Get(ctx context.Context, id uuid.UUID) (GetRow, error)
	List(ctx context.Context, status Status, visibility Visibility, excludeExpired bool) ([]ListRow, error)
	ListByUnit(ctx context.Context, arg ListByUnitParams) ([]ListByUnitRow, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status, userID uuid.UUID) (Form, error)
	UploadCoverImage(ctx context.Context, id uuid.UUID, data []byte, coverImageURL string) error
	GetCoverImage(ctx context.Context, id uuid.UUID) ([]byte, error)
}

type tenantStore interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
}

type questionStore interface {
	UpdateSection(ctx context.Context, arg question.UpdateSectionParams) (question.Section, error)
	Get(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type FileStore interface {
	SaveFile(ctx context.Context, fileContent io.Reader, originalFilename, contentType string, uploadedBy *uuid.UUID, opts ...file.ValidatorOption) (file.File, error)
	Get(ctx context.Context, id uuid.UUID) (file.File, error)
}

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store         Store
	tenantStore   tenantStore
	questionStore questionStore
	fileStore     FileStore
	markdownStore MarkdownStore
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	tenantStore tenantStore,
	questionStore questionStore,
	fileStore FileStore,
	markdownStore MarkdownStore,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("form/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tenantStore:   tenantStore,
		questionStore: questionStore,
		fileStore:     fileStore,
		markdownStore: markdownStore,
	}
}

func formFromCreateRow(r CreateRow) Form {
	return Form{
		ID:                     r.ID,
		Title:                  r.Title,
		DescriptionJson:        r.DescriptionJson,
		DescriptionHtml:        r.DescriptionHtml,
		PreviewMessage:         r.PreviewMessage,
		Status:                 r.Status,
		UnitID:                 r.UnitID,
		CreatedBy:              r.CreatedBy,
		LastEditor:             r.LastEditor,
		Deadline:               r.Deadline,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		MessageAfterSubmission: r.MessageAfterSubmission,
		Visibility:             r.Visibility,
		GoogleSheetUrl:         r.GoogleSheetUrl,
		PublishTime:            r.PublishTime,
		CoverImageUrl:          r.CoverImageUrl,
		DressingColor:          r.DressingColor,
		DressingHeaderFont:     r.DressingHeaderFont,
		DressingQuestionFont:   r.DressingQuestionFont,
		DressingTextFont:       r.DressingTextFont,
	}
}

func formFromGetRow(r GetRow) Form {
	return Form{
		ID:                     r.ID,
		Title:                  r.Title,
		DescriptionJson:        r.DescriptionJson,
		DescriptionHtml:        r.DescriptionHtml,
		PreviewMessage:         r.PreviewMessage,
		Status:                 r.Status,
		UnitID:                 r.UnitID,
		CreatedBy:              r.CreatedBy,
		LastEditor:             r.LastEditor,
		Deadline:               r.Deadline,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		MessageAfterSubmission: r.MessageAfterSubmission,
		Visibility:             r.Visibility,
		GoogleSheetUrl:         r.GoogleSheetUrl,
		PublishTime:            r.PublishTime,
		CoverImageUrl:          r.CoverImageUrl,
		DressingColor:          r.DressingColor,
		DressingHeaderFont:     r.DressingHeaderFont,
		DressingQuestionFont:   r.DressingQuestionFont,
		DressingTextFont:       r.DressingTextFont,
	}
}

func formFromPatchRow(r PatchRow) Form {
	return Form{
		ID:                     r.ID,
		Title:                  r.Title,
		DescriptionJson:        r.DescriptionJson,
		DescriptionHtml:        r.DescriptionHtml,
		PreviewMessage:         r.PreviewMessage,
		Status:                 r.Status,
		UnitID:                 r.UnitID,
		CreatedBy:              r.CreatedBy,
		LastEditor:             r.LastEditor,
		Deadline:               r.Deadline,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		MessageAfterSubmission: r.MessageAfterSubmission,
		Visibility:             r.Visibility,
		GoogleSheetUrl:         r.GoogleSheetUrl,
		PublishTime:            r.PublishTime,
		CoverImageUrl:          r.CoverImageUrl,
		DressingColor:          r.DressingColor,
		DressingHeaderFont:     r.DressingHeaderFont,
		DressingQuestionFont:   r.DressingQuestionFont,
		DressingTextFont:       r.DressingTextFont,
	}
}

func formFromListRow(r ListRow) Form {
	return Form{
		ID:                     r.ID,
		Title:                  r.Title,
		DescriptionJson:        r.DescriptionJson,
		DescriptionHtml:        r.DescriptionHtml,
		PreviewMessage:         r.PreviewMessage,
		Status:                 r.Status,
		UnitID:                 r.UnitID,
		CreatedBy:              r.CreatedBy,
		LastEditor:             r.LastEditor,
		Deadline:               r.Deadline,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		MessageAfterSubmission: r.MessageAfterSubmission,
		Visibility:             r.Visibility,
		GoogleSheetUrl:         r.GoogleSheetUrl,
		PublishTime:            r.PublishTime,
		CoverImageUrl:          r.CoverImageUrl,
		DressingColor:          r.DressingColor,
		DressingHeaderFont:     r.DressingHeaderFont,
		DressingQuestionFont:   r.DressingQuestionFont,
		DressingTextFont:       r.DressingTextFont,
	}
}

func formFromListByUnitRow(r ListByUnitRow) Form {
	return Form{
		ID:                     r.ID,
		Title:                  r.Title,
		DescriptionJson:        r.DescriptionJson,
		DescriptionHtml:        r.DescriptionHtml,
		PreviewMessage:         r.PreviewMessage,
		Status:                 r.Status,
		UnitID:                 r.UnitID,
		CreatedBy:              r.CreatedBy,
		LastEditor:             r.LastEditor,
		Deadline:               r.Deadline,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
		MessageAfterSubmission: r.MessageAfterSubmission,
		Visibility:             r.Visibility,
		GoogleSheetUrl:         r.GoogleSheetUrl,
		PublishTime:            r.PublishTime,
		CoverImageUrl:          r.CoverImageUrl,
		DressingColor:          r.DressingColor,
		DressingHeaderFont:     r.DressingHeaderFont,
		DressingQuestionFont:   r.DressingQuestionFont,
		DressingTextFont:       r.DressingTextFont,
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

	response := ToResponse(
		formFromPatchRow(currentForm),
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		UserFromProfileFields(currentForm.CreatedBy, currentForm.CreatorName, currentForm.CreatorUsername, currentForm.CreatorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.CreatorEmails),
		UserFromProfileFields(currentForm.LastEditor, currentForm.LastEditorName, currentForm.LastEditorUsername, currentForm.LastEditorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail),
	)
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

	currentForm, err := h.store.Get(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(
		formFromGetRow(currentForm),
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		UserFromProfileFields(currentForm.CreatedBy, currentForm.CreatorName, currentForm.CreatorUsername, currentForm.CreatorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.CreatorEmails),
		UserFromProfileFields(currentForm.LastEditor, currentForm.LastEditorName, currentForm.LastEditorUsername, currentForm.LastEditorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail),
	)
	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	forms, err := h.store.List(traceCtx, "", "", false)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, 0, len(forms))
	for _, form := range forms {
		responses = append(responses, ToResponse(
			formFromListRow(form),
			form.UnitName.String,
			form.OrgName.String,
			UserFromProfileFields(form.CreatedBy, form.CreatorName, form.CreatorUsername, form.CreatorAvatarUrl),
			user.ConvertEmailsToSlice(form.CreatorEmails),
			UserFromProfileFields(form.LastEditor, form.LastEditorName, form.LastEditorUsername, form.LastEditorAvatarUrl),
			user.ConvertEmailsToSlice(form.LastEditorEmail),
		))
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

	response := ToResponse(
		formFromCreateRow(newForm),
		newForm.UnitName.String,
		newForm.OrgName.String,
		UserFromProfileFields(newForm.CreatedBy, newForm.CreatorName, newForm.CreatorUsername, newForm.CreatorAvatarUrl),
		user.ConvertEmailsToSlice(newForm.CreatorEmails),
		UserFromProfileFields(newForm.LastEditor, newForm.LastEditorName, newForm.LastEditorUsername, newForm.LastEditorAvatarUrl),
		user.ConvertEmailsToSlice(newForm.LastEditorEmail),
	)
	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
}

func (h *Handler) ListByOrgHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListByOrgHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var status []Status
	statusStr := r.URL.Query()["status"]
	if len(statusStr) == 0 {
		status = []Status{StatusDraft, StatusPublished}
	} else {
		for _, s := range statusStr {
			parseStatus, err := ParseStatus(s)
			if err != nil {
				h.problemWriter.WriteError(traceCtx, w, err, logger)
				return
			}

			status = append(status, parseStatus)
		}
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

	forms, err := h.store.ListByUnit(traceCtx, ListByUnitParams{
		UnitID: pgtype.UUID{Bytes: orgID, Valid: true},
		Status: status,
	})
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, len(forms))
	for i, currentForm := range forms {
		responses[i] = ToResponse(
			formFromListByUnitRow(currentForm),
			currentForm.UnitName.String,
			currentForm.OrgName.String,
			UserFromProfileFields(currentForm.CreatedBy, currentForm.CreatorName, currentForm.CreatorUsername, currentForm.CreatorAvatarUrl),
			user.ConvertEmailsToSlice(currentForm.CreatorEmails),
			UserFromProfileFields(currentForm.LastEditor, currentForm.LastEditorName, currentForm.LastEditorUsername, currentForm.LastEditorAvatarUrl),
			user.ConvertEmailsToSlice(currentForm.LastEditorEmail),
		)
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
	savedFile, err := h.fileStore.SaveFile(
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

	currentForm, err := h.store.Get(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(
		formFromGetRow(currentForm),
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		UserFromProfileFields(currentForm.CreatedBy, currentForm.CreatorName, currentForm.CreatorUsername, currentForm.CreatorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.CreatorEmails),
		UserFromProfileFields(currentForm.LastEditor, currentForm.LastEditorName, currentForm.LastEditorUsername, currentForm.LastEditorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail),
	)

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) UnarchiveHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UnarchiveHandler")
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

	_, err = h.store.SetStatus(traceCtx, id, StatusDraft, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentForm, err := h.store.Get(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response := ToResponse(
		formFromGetRow(currentForm),
		currentForm.UnitName.String,
		currentForm.OrgName.String,
		UserFromProfileFields(currentForm.CreatedBy, currentForm.CreatorName, currentForm.CreatorUsername, currentForm.CreatorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.CreatorEmails),
		UserFromProfileFields(currentForm.LastEditor, currentForm.LastEditorName, currentForm.LastEditorUsername, currentForm.LastEditorAvatarUrl),
		user.ConvertEmailsToSlice(currentForm.LastEditorEmail),
	)

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

	params := question.UpdateSectionParams{
		ID:     sectionID,
		FormID: formID,
		Title:  pgtype.Text{String: req.Title, Valid: true},
	}
	if req.Description.Set {
		j, htmlStr, err := h.markdownStore.ProcessAPIText(traceCtx, req.Description.Value)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}
		params.DescriptionJson = j
		params.DescriptionHtml = pgtype.Text{String: htmlStr, Valid: true}
	}

	section, err := h.questionStore.UpdateSection(traceCtx, params)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	secDesc := markdown.DefaultDescriptionJSON(section.DescriptionJson)
	response := SectionResponse{
		ID:              section.ID.String(),
		FormID:          section.FormID.String(),
		Title:           section.Title.String,
		Description:     secDesc,
		DescriptionHTML: section.DescriptionHtml,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func ParseStatus(status string) (Status, error) {
	switch status {
	case "DRAFT":
		return StatusDraft, nil
	case "PUBLISHED":
		return StatusPublished, nil
	case "ARCHIVED":
		return StatusArchived, nil
	default:
		return "", internal.ErrInvalidStatus
	}
}
