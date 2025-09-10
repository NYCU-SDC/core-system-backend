package unit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
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

type Store interface {
	GetOrgIDBySlug(ctx context.Context, slug string) (uuid.UUID, error)
	CreateUnit(ctx context.Context, name string, orgID uuid.UUID, desc string, metadata []byte) (Unit, error)
	CreateOrg(ctx context.Context, name string, desc string, creatorID uuid.UUID, metadata []byte, slug string) (Organization, error)
	GetByID(ctx context.Context, id uuid.UUID, orgID uuid.UUID, unitType Type) (GenericUnit, error)
	GetAllOrganizations(ctx context.Context) ([]Organization, error)
	UpdateUnit(ctx context.Context, id uuid.UUID, name string, description string, metadata []byte) (Unit, error)
	UpdateOrg(ctx context.Context, id uuid.UUID, name string, description string, metadata []byte, slug string) (Organization, error)
	Delete(ctx context.Context, id uuid.UUID, unitType Type) error
	AddParentChild(ctx context.Context, parentID uuid.UUID, childID uuid.UUID, orgID uuid.UUID) (ParentChild, error)
	RemoveParentChild(ctx context.Context, childID uuid.UUID) error
	ListSubUnits(ctx context.Context, id uuid.UUID, unitType Type) ([]Unit, error)
	ListSubUnitIDs(ctx context.Context, id uuid.UUID, unitType Type) ([]uuid.UUID, error)
}

type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	formService   *form.Service
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	formService *form.Service,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		formService:   formService,
		tracer:        otel.Tracer("unit/handler"),
	}
}

type OrgRequest struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	Slug        string            `json:"slug" validate:"required"`
}

type Request struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

type orgResponse struct {
	ID          uuid.UUID         `json:"id"`
	OwnerID     uuid.UUID         `json:"owner_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	Slug        string            `json:"slug"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type Response struct {
	ID          uuid.UUID         `json:"id"`
	OrgID       uuid.UUID         `json:"org_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

func convertResponse(u Unit) Response {
	var meta map[string]string
	if err := json.Unmarshal(u.Metadata, &meta); err != nil {
		meta = make(map[string]string)
	}
	return Response{
		ID:          u.ID,
		OrgID:       u.OrgID,
		Name:        u.Name.String,
		Description: u.Description.String,
		Metadata:    meta,
		CreatedAt:   u.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   u.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func convertOrgResponse(o Organization) orgResponse {
	var meta map[string]string
	if err := json.Unmarshal(o.Metadata, &meta); err != nil {
		meta = make(map[string]string)
	}
	return orgResponse{
		ID:          o.ID,
		OwnerID:     o.OwnerID.Bytes,
		Name:        o.Name.String,
		Description: o.Description.String,
		Metadata:    meta,
		Slug:        o.Slug,
		CreatedAt:   o.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   o.UpdatedAt.Time.Format(time.RFC3339),
	}
}

type ParentChildRequest struct {
	ParentID uuid.UUID `json:"parent_id"`
	ChildID  uuid.UUID `json:"child_id" validate:"required"`
	OrgID    uuid.UUID `json:"org_id" validate:"required"`
}

func (h *Handler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateUnit")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req Request

	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), h.logger)
		return
	}

	orgSlug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgID, err := h.store.GetOrgIDBySlug(traceCtx, orgSlug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	createdUnit, err := h.store.CreateUnit(traceCtx, req.Name, orgID, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertResponse(createdUnit))
}

func (h *Handler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateOrg")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req OrgRequest

	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), h.logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("no user found in request context"), h.logger)
		return
	}

	createdOrg, err := h.store.CreateOrg(traceCtx, req.Name, req.Description, currentUser.ID, metadataBytes, req.Slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertOrgResponse(createdOrg))
}

func (h *Handler) GetUnitByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetUnitByID")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	idStr := r.PathValue("id")

	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	orgID, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	unitWrap, err := h.store.GetByID(traceCtx, id, orgID, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get unit by ID: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertResponse(unitWrap.Instance().(Unit)))
}

func (h *Handler) GetOrgByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetOrgByID")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgID, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	orgWrap, err := h.store.GetByID(traceCtx, orgID, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get unit by ID: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(orgWrap.Instance().(Organization)))
}

func (h *Handler) GetAllOrganizations(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAllOrganizations")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	organizations, err := h.store.GetAllOrganizations(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get all organizations: %w", err), h.logger)
		return
	}

	orgResponses := make([]orgResponse, 0)
	for _, org := range organizations {
		orgResponses = append(orgResponses, convertOrgResponse(org))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, orgResponses)
}

func (h *Handler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateUnit")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), h.logger)
		return
	}

	updatedUnit, err := h.store.UpdateUnit(traceCtx, id, req.Name, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertResponse(updatedUnit))
}

func (h *Handler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateOrg")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req OrgRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	id, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), h.logger)
		return
	}

	updatedOrg, err := h.store.UpdateOrg(traceCtx, id, req.Name, req.Description, metadataBytes, req.Slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update organization: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(updatedOrg))
}

func (h *Handler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteOrg")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	id, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	err = h.store.Delete(traceCtx, id, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to delete unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

// DeleteUnit deletes a unit by its ID
func (h *Handler) DeleteUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteUnit")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	err = h.store.Delete(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to delete unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) AddParentChild(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddParentChild")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req ParentChildRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	pc, err := h.store.AddParentChild(traceCtx, req.ParentID, req.ChildID, req.OrgID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add parent-child relationship: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, pc)
}

func (h *Handler) RemoveParentChild(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RemoveParentChild")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	cIDStr := r.PathValue("child_id")
	if cIDStr == "" {
		http.Error(w, "parent or child ID not provided", http.StatusBadRequest)
		return
	}
	cID, err := internal.ParseUUID(cIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	err = h.store.RemoveParentChild(traceCtx, cID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to remove parent-child relationship: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) ListOrgSubUnits(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListOrgSubUnits")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgID, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	subUnits, err := h.store.ListSubUnits(traceCtx, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), h.logger)
		return
	}

	responses := make([]Response, 0)
	for _, u := range subUnits {
		responses = append(responses, convertResponse(u))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

func (h *Handler) ListUnitSubUnits(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListUnitSubUnits")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}
	subUnits, err := h.store.ListSubUnits(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), h.logger)
		return
	}

	responses := make([]Response, 0)
	for _, u := range subUnits {
		responses = append(responses, convertResponse(u))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, responses)
}

func (h *Handler) ListOrgSubUnitIDs(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListOrgSubUnits")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgID, err := h.store.GetOrgIDBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	subUnits, err := h.store.ListSubUnitIDs(traceCtx, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, subUnits)
}

func (h *Handler) ListUnitSubUnitIDs(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListUnitSubUnits")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	subUnits, err := h.store.ListSubUnitIDs(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, subUnits)
}

func (h *Handler) CreateFormUnderUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateFormHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req form.Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	unitIDStr := r.PathValue("unitId")
	currentUnitID, err := handlerutil.ParseUUID(unitIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	newForm, err := h.formService.Create(traceCtx, req, currentUnitID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, newForm)
}

func (h *Handler) ListFormsByUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListFormsByUnitHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	unitIDStr := r.PathValue("unitId")
	unitID, err := handlerutil.ParseUUID(unitIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	forms, err := h.formService.ListByUnit(traceCtx, unitID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, forms)
}
