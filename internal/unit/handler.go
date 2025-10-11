package unit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/tenant"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	CreateOrganization(ctx context.Context, name string, description string, metadata []byte) (Unit, error)
	CreateUnit(ctx context.Context, name string, orgID pgtype.UUID, desc string, metadata []byte) (Unit, error)
	GetByID(ctx context.Context, id uuid.UUID, unitType Type) (Unit, error)
	GetAllOrganizations(ctx context.Context) ([]Organization, error)
	Update(ctx context.Context, id uuid.UUID, name string, description string, metadata []byte) (Unit, error)
	Delete(ctx context.Context, id uuid.UUID, unitType Type) error
	AddParent(ctx context.Context, id uuid.UUID, parentID uuid.UUID) (Unit, error)
	ListSubUnits(ctx context.Context, id uuid.UUID, unitType Type) ([]Unit, error)
	ListSubUnitIDs(ctx context.Context, id uuid.UUID, unitType Type) ([]uuid.UUID, error)
	AddMember(ctx context.Context, unitType Type, id uuid.UUID, username string) (AddMemberRow, error)
	ListMembers(ctx context.Context, id uuid.UUID) ([]user.Profile, error)
	RemoveMember(ctx context.Context, unitType Type, id uuid.UUID, memberID uuid.UUID) error
	GetOrganizationByIDWithSlug(ctx context.Context, id uuid.UUID) (Organization, error)
}

type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	tenantService *tenant.Service
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	tenantService *tenant.Service,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		tenantService: tenantService,
		tracer:        otel.Tracer("unit/handler"),
	}
}

type OrgRequest struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	Slug        string            `json:"slug" validate:"required"`
	DbStrategy  string            `json:"dbStrategy"`
}

type Request struct {
	Name        string            `json:"name" validate:"required"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

type UnitResponse struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

type OrganizationResponse struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
	Slug        string            `json:"slug"`
}

type OrgMemberResponse struct {
	OrgID      uuid.UUID            `json:"orgId"`
	SimpleUser user.ProfileResponse `json:"member"`
}

type UnitMemberResponse struct {
	UnitID     uuid.UUID            `json:"unitId"`
	SimpleUser user.ProfileResponse `json:"member"`
}

func convertUnitResponse(u Unit) UnitResponse {
	var meta map[string]string
	if err := json.Unmarshal(u.Metadata, &meta); err != nil {
		meta = make(map[string]string)
	}

	return UnitResponse{
		ID:          u.ID,
		Name:        u.Name.String,
		Description: u.Description.String,
		Metadata:    meta,
		CreatedAt:   u.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   u.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func convertOrgResponse(u Unit, slug string) OrganizationResponse {
	var meta map[string]string
	if err := json.Unmarshal(u.Metadata, &meta); err != nil {
		meta = make(map[string]string)
	}

	return OrganizationResponse{
		ID:          u.ID,
		Name:        u.Name.String,
		Description: u.Description.String,
		Metadata:    meta,
		CreatedAt:   u.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   u.UpdatedAt.Time.Format(time.RFC3339),
		Slug:        slug,
	}
}

type parentChildResponse struct {
	ParentID *uuid.UUID `json:"parentId,omitempty"`
	ChildID  uuid.UUID  `json:"childId"`
	OrgID    uuid.UUID  `json:"orgId"`
}

type ParentChildRequest struct {
	ParentID uuid.UUID `json:"parentId"`
	ChildID  uuid.UUID `json:"childId" validate:"required"`
	OrgID    uuid.UUID `json:"orgId" validate:"required"`
}

var slugPattern = `^[a-zA-Z0-9_-]+$`

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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, orgSlug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	createdUnit, err := h.store.CreateUnit(traceCtx, req.Name, pgtype.UUID{Bytes: orgTenant.ID, Valid: true}, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertUnitResponse(createdUnit))
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

	matched, err := regexp.MatchString(slugPattern, req.Slug)
	if err != nil || !matched {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid slug format: must contain only alphanumeric characters, dashes, and underscores"), h.logger)
		return
	}

	exists, err := h.tenantService.SlugExists(traceCtx, req.Slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to validate slug uniqueness: %w", err), h.logger)
		return
	}
	if exists {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("slug already in use"), h.logger)
		return
	}

	createdOrg, err := h.store.CreateOrganization(traceCtx, req.Name, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create org: %w", err), h.logger)
		return
	}

	_, err = h.tenantService.Create(traceCtx, req.Slug, createdOrg.ID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create tenant for org: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertOrgResponse(createdOrg, req.Slug))
}

func (h *Handler) GetUnitByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetUnitByID")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")

	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, h.logger)
		return
	}

	unit, err := h.store.GetByID(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get unit by ID: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertUnitResponse(unit))
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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	orgWithSlug, err := h.store.GetOrganizationByIDWithSlug(traceCtx, orgTenant.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get organization by ID with slug: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(orgWithSlug.Unit, orgWithSlug.Slug))
}

func (h *Handler) GetAllOrganizations(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAllOrganizations")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	organizationsWithSlug, err := h.store.GetAllOrganizations(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get all organizations: %w", err), h.logger)
		return
	}

	orgResponses := make([]OrganizationResponse, 0, len(organizationsWithSlug))
	for _, org := range organizationsWithSlug {
		orgResponses = append(orgResponses, convertOrgResponse(org.Unit, org.Slug))
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

	updatedUnit, err := h.store.Update(traceCtx, id, req.Name, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update unit: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertUnitResponse(updatedUnit))
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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	if req.Slug != slug {
		matched, err := regexp.MatchString(slugPattern, req.Slug)
		if err != nil || !matched {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid slug format: must contain only alphanumeric characters, dashes, and underscores"), h.logger)
			return
		}

		exists, err := h.tenantService.SlugExists(traceCtx, req.Slug)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to validate slug uniqueness: %w", err), h.logger)
			return
		}
		if exists {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("slug already in use"), h.logger)
			return
		}
	}
	// TODO: Slug Validator

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), h.logger)
		return
	}

	var dbStrategy tenant.DbStrategy

	if req.DbStrategy == "" || req.DbStrategy == string(DbStrategyShared) {
		dbStrategy = "shared"
	} else if req.DbStrategy == string(DbStrategyIsolated) {
		dbStrategy = "isolated"
	}

	_, err = h.tenantService.Update(traceCtx, orgTenant.ID, req.Slug, dbStrategy)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update organization tenant: %w", err), h.logger)
		return
	}

	updatedOrg, err := h.store.Update(traceCtx, orgTenant.ID, req.Name, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update organization: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(updatedOrg, req.Slug))
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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	err = h.store.Delete(traceCtx, orgTenant.ID, TypeOrg)
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
	traceCtx, span := h.tracer.Start(r.Context(), "AddParent")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	var req ParentChildRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	pc, err := h.store.AddParent(traceCtx, req.ParentID, req.ChildID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add parent-child relationship: %w", err), h.logger)
		return
	}

	var parent *uuid.UUID
	if req.ParentID != uuid.Nil {
		pid := req.ParentID
		parent = &pid
	}
	response := parentChildResponse{
		ParentID: parent,
		ChildID:  pc.ID,
		OrgID:    pc.OrgID.Bytes,
	}
	handlerutil.WriteJSONResponse(w, http.StatusCreated, response)
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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	subUnits, err := h.store.ListSubUnits(traceCtx, orgTenant.ID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), h.logger)
		return
	}

	responses := make([]UnitResponse, 0)
	for _, u := range subUnits {
		responses = append(responses, convertUnitResponse(u))
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

	responses := make([]UnitResponse, 0)
	for _, u := range subUnits {
		responses = append(responses, convertUnitResponse(u))
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

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	subUnits, err := h.store.ListSubUnitIDs(traceCtx, orgTenant.ID, TypeOrg)
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

func (h *Handler) AddOrgMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddOrgMember")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	// Get Username from request body
	var params struct {
		Email string `json:"email"`
	}
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &params); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	if orgTenant.ID == uuid.Nil || params.Email == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("org ID or member username cannot be empty"), h.logger)
		return
	}

	members, err := h.store.AddMember(traceCtx, TypeOrg, orgTenant.ID, params.Email)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add org member: %w", err), h.logger)
		return
	}

	orgMemberResponse := OrgMemberResponse{
		OrgID: orgTenant.ID,
		SimpleUser: user.ProfileResponse{
			ID:        members.MemberID,
			Name:      members.Name.String,
			Username:  members.Username.String,
			AvatarURL: members.AvatarUrl.String,
			Email:     members.Email,
		},
	}
	handlerutil.WriteJSONResponse(w, http.StatusCreated, orgMemberResponse)
}

func (h *Handler) AddUnitMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddUnitMember")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), h.logger)
		return
	}

	var params struct {
		Email string `json:"email"`
	}
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &params); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), h.logger)
		return
	}

	if params.Email == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("member username cannot be empty"), h.logger)
		return
	}

	member, err := h.store.AddMember(traceCtx, TypeUnit, id, params.Email)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add unit member: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, UnitMemberResponse{
		UnitID: id,
		SimpleUser: user.ProfileResponse{
			ID:        member.MemberID,
			Name:      member.Name.String,
			Username:  member.Username.String,
			Email:     member.Email,
			AvatarURL: member.AvatarUrl.String,
		},
	})
}

func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListOrgMembers")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	// Todo: Need to recursively obtain members of the entire organization
	members, err := h.store.ListMembers(traceCtx, orgTenant.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list org members: %w", err), h.logger)
		return
	}

	response := make([]user.ProfileResponse, 0, len(members))
	for _, m := range members {
		response = append(response, user.ProfileResponse{
			ID:        m.ID,
			Name:      m.Name,
			Username:  m.Username,
			Email:     m.Email,
			AvatarURL: m.AvatarURL,
		})
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) ListUnitMembers(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListUnitMembers")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), h.logger)
		return
	}

	members, err := h.store.ListMembers(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list unit members: %w", err), h.logger)
		return
	}

	response := make([]user.ProfileResponse, 0, len(members))
	for _, m := range members {
		response = append(response, user.ProfileResponse{
			ID:        m.ID,
			Name:      m.Name,
			Username:  m.Username,
			Email:     m.Email,
			AvatarURL: m.AvatarURL,
		})
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) RemoveOrgMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RemoveOrgMember")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), h.logger)
		return
	}

	orgTenant, err := h.tenantService.GetBySlug(traceCtx, slug)
	if err != nil || orgTenant.ID == uuid.Nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), h.logger)
		return
	}

	mIDStr := r.PathValue("member_id")

	if mIDStr == "" {
		http.Error(w, "member ID not provided", http.StatusBadRequest)
		return
	}
	mID, err := uuid.Parse(mIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid member ID: %w", err), h.logger)
		return
	}

	err = h.store.RemoveMember(traceCtx, TypeOrg, orgTenant.ID, mID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to remove org member: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) RemoveUnitMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RemoveUnitMember")
	defer span.End()
	h.logger = logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), h.logger)
		return
	}

	mIDStr := r.PathValue("member_id")
	if mIDStr == "" {
		http.Error(w, "member ID not provided", http.StatusBadRequest)
		return
	}

	mID, err := uuid.Parse(mIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid member ID: %w", err), h.logger)
		return
	}

	err = h.store.RemoveMember(traceCtx, TypeUnit, id, mID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to remove unit member: %w", err), h.logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}
