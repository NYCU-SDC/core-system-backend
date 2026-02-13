package form

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/font"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	Description            string           `json:"description" validate:"required"`
	PreviewMessage         string           `json:"previewMessage"`
	Deadline               *time.Time       `json:"deadline"`
	PublishTime            *time.Time       `json:"publishTime"`
	MessageAfterSubmission string           `json:"messageAfterSubmission" validate:"required"`
	GoogleSheetUrl         string           `json:"googleSheetUrl"`
	Visibility             Visibility       `json:"visibility" validate:"required,oneof=public private"`
	CoverImageUrl          string           `json:"coverImageUrl"`
	Dressing               *DressingRequest `json:"dressing"`
}

type Response struct {
	ID                     string               `json:"id"`
	Title                  string               `json:"title"`
	Description            string               `json:"description"`
	PreviewMessage         string               `json:"previewMessage"`
	Status                 string               `json:"status"`
	UnitID                 string               `json:"unitId"`
	OrgID                  string               `json:"orgId"`
	LastEditor             user.ProfileResponse `json:"lastEditor"`
	Deadline               *time.Time           `json:"deadline"`
	CreatedAt              time.Time            `json:"createdAt"`
	UpdatedAt              time.Time            `json:"updatedAt"`
	PublishTime            *time.Time           `json:"publishTime"`
	MessageAfterSubmission string               `json:"messageAfterSubmission"`
	GoogleSheetUrl         string               `json:"googleSheetUrl"`
	Visibility             Visibility           `json:"visibility"`
	CoverImageUrl          string               `json:"coverImageUrl"`
	Dressing               DressingRequest      `json:"dressing"`
}

type CoverUploadResponse struct {
	ImageURL string `json:"imageUrl"`
}
type GoogleSheetEmailResponse struct {
	Email string `json:"email"`
}
type GoogleSheetVerifyRequest struct {
	GoogleSheetURL string `json:"googleSheetUrl" validate:"required"`
}

type GoogleSheetVerifyResponse struct {
	IsValid bool `json:"isValid"`
}

type emailGetter interface {
	GetServiceAccountEmail() string
}

type verifier interface {
	VerifySpreadsheetReadable(ctx context.Context, spreadsheetID string) error
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
		Status:         string(form.Status),
		UnitID:         unitName,
		OrgID:          orgName,
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
		Visibility:             form.Visibility,
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
	Update(ctx context.Context, id uuid.UUID, request Request, userID uuid.UUID) (UpdateRow, error)
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

type Handler struct {
	logger *zap.Logger
	tracer trace.Tracer

	validator     *validator.Validate
	problemWriter *problem.HttpWriter

	store       Store
	tenantStore tenantStore
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	tenantStore tenantStore,
) *Handler {
	return &Handler{
		logger:        logger,
		tracer:        otel.Tracer("form/handler"),
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tenantStore:   tenantStore,
	}
}

func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

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

	currentForm, err := h.store.Update(traceCtx, id, req, currentUser.ID)
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

	idStr := r.PathValue("id")
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

	idStr := r.PathValue("id")
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

	idStr := r.PathValue("id")
	id, err := handlerutil.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	const maxBytes int64 = 2 << 20 // 2MB

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	file, _, err := r.FormFile("coverImage")
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			logutil.WithContext(traceCtx, logger).Warn(
				"failed to close cover image file",
				zap.String("form_id", id.String()),
				zap.Error(err),
			)
		}
	}()

	imageBytes, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to read cover image: %w", err), logger)
		return
	}
	if int64(len(imageBytes)) > maxBytes {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrCoverImageTooLarge, logger)
		return
	}

	// WebP validation
	if len(imageBytes) < 12 ||
		string(imageBytes[0:4]) != "RIFF" ||
		string(imageBytes[8:12]) != "WEBP" {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrCoverImageInvalidFormat, logger)
		return
	}

	coverImageURL := fmt.Sprintf("/api/forms/%s/cover", id.String())

	if err := h.store.UploadCoverImage(traceCtx, id, imageBytes, coverImageURL); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, CoverUploadResponse{ImageURL: coverImageURL})
}

func (h *Handler) GetCoverImageHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetCoverImageHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
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

func (h *Handler) GetGoogleSheetEmailHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetGoogleSheetEmailHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	getter, ok := h.store.(emailGetter)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	email := getter.GetServiceAccountEmail()
	if email == "" {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, GoogleSheetEmailResponse{Email: email})
}

func (h *Handler) VerifyGoogleSheetHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "VerifyGoogleSheetHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req GoogleSheetVerifyRequest
	err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	spreadsheetID, err := extractSpreadsheetID(req.GoogleSheetURL)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	v, ok := h.store.(verifier)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	err = v.VerifySpreadsheetReadable(traceCtx, spreadsheetID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, GoogleSheetVerifyResponse{IsValid: true})
}

func extractSpreadsheetID(sheetURL string) (string, error) {
	u, err := url.Parse(sheetURL)
	if err != nil {
		return "", internal.ErrGoogleSheetURLInvalid
	}

	parts := strings.Split(u.Path, "/")
	// /spreadsheets/d/<id>/edit
	for i := 0; i < len(parts); i++ {
		if parts[i] == "spreadsheets" && i+2 < len(parts) && parts[i+1] == "d" {
			id := parts[i+2]
			if id != "" {
				return id, nil
			}
		}
	}
	return "", internal.ErrGoogleSheetURLInvalid
}
