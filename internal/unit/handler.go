package unit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/tenant"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
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
	CreateOrganization(ctx context.Context, name string, description string, slug string, currentUserID uuid.UUID, metadata []byte) (Unit, error)
	CreateUnit(ctx context.Context, name string, description string, slug string, metadata []byte) (Unit, error)
	GetByID(ctx context.Context, id uuid.UUID, unitType Type) (Unit, error)
	GetAllOrganizations(ctx context.Context) ([]Organization, error)
	ListOrganizationsOfUser(ctx context.Context, userID uuid.UUID) ([]Organization, error)
	UpdateOrg(ctx context.Context, originalSlug string, slug string, name string, description string, dbStrategy string, metadata []byte) (Unit, error)
	UpdateUnit(ctx context.Context, id uuid.UUID, name string, description string, metadata []byte) (Unit, error)
	Delete(ctx context.Context, id uuid.UUID, unitType Type) error
	AddParent(ctx context.Context, id uuid.UUID, parentID uuid.UUID) (Unit, error)
	ListSubUnits(ctx context.Context, id uuid.UUID, unitType Type) ([]Unit, error)
	ListSubUnitIDs(ctx context.Context, id uuid.UUID, unitType Type) ([]uuid.UUID, error)
	AddMember(ctx context.Context, unitType Type, id uuid.UUID, username string) (AddMemberRow, error)
	ListMembers(ctx context.Context, id uuid.UUID) ([]user.Profile, error)
	RemoveMember(ctx context.Context, unitType Type, id uuid.UUID, memberID uuid.UUID) error
	GetOrganizationByIDWithSlug(ctx context.Context, id uuid.UUID) (Organization, error)
	UpdateUnitMemberRole(ctx context.Context, unitID uuid.UUID, memberID uuid.UUID, newRole UnitRole) error
}

type formStore interface {
	Create(ctx context.Context, req form.Request, unitID uuid.UUID, userID uuid.UUID) (form.CreateRow, error)
	ListByUnit(context.Context, uuid.UUID) ([]form.ListByUnitRow, error)
	ListFormsOfUser(ctx context.Context, unitIDs []uuid.UUID, userID uuid.UUID) ([]form.UserForm, error)
}

type tenantStore interface {
	GetSlugStatus(ctx context.Context, slug string) (bool, uuid.UUID, error)
	Create(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, slug string) (tenant.Tenant, error)
	Update(ctx context.Context, id uuid.UUID, slug string, dbStrategy tenant.DbStrategy) (tenant.Tenant, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
}

type userStore interface {
	GetEmailsByID(ctx context.Context, userID uuid.UUID) ([]string, error)
}
type Handler struct {
	logger        *zap.Logger
	tracer        trace.Tracer
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	formStore     formStore
	tenantStore   tenantStore
	userStore     userStore
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	formStore formStore,
	tenantStore tenantStore,
	userStore userStore,
) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		formStore:     formStore,
		tenantStore:   tenantStore,
		userStore:     userStore,
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

type UserFormResponse struct {
	FormID   string              `json:"id"`
	Title    string              `json:"title"`
	Deadline *time.Time          `json:"deadline"`
	Status   form.UserFormStatus `json:"status"`
}

// createProfileResponseWithEmails creates a ProfileResponse with emails for a user
func (h *Handler) createProfileResponseWithEmails(ctx context.Context, logger *zap.Logger, userID uuid.UUID, name, username, avatarURL string) user.ProfileResponse {
	emails, err := h.userStore.GetEmailsByID(ctx, userID)
	if err != nil {
		// Log the error but don't fail the request
		logger.Warn("failed to get user emails", zap.Error(err), zap.String("user_id", userID.String()))
		emails = []string{}
	}

	return user.ProfileResponse{
		ID:        userID,
		Name:      name,
		Username:  username,
		AvatarURL: avatarURL,
		Emails:    emails,
	}
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
	logger := logutil.WithContext(traceCtx, h.logger)

	var req Request

	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), logger)
		return
	}

	orgSlug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), logger)
		return
	}

	createdUnit, err := h.store.CreateUnit(traceCtx, req.Name, req.Description, orgSlug, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create unit: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertUnitResponse(createdUnit))
}

func (h *Handler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateOrg")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req OrgRequest

	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("no user found in request context"), logger)
		return
	}

	matched, err := regexp.MatchString(slugPattern, req.Slug)
	if err != nil || !matched {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid slug format: must contain only alphanumeric characters, dashes, and underscores"), logger)
		return
	}

	createdOrg, err := h.store.CreateOrganization(traceCtx, req.Name, req.Description, req.Slug, currentUser.ID, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to create org: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, convertOrgResponse(createdOrg, req.Slug))
}

func (h *Handler) GetUnitByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetUnitByID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")

	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	unit, err := h.store.GetByID(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get unit by ID: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertUnitResponse(unit))
}

func (h *Handler) GetOrgByID(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetOrgByID")
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

	orgWithSlug, err := h.store.GetOrganizationByIDWithSlug(traceCtx, orgID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get organization by ID with slug: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(orgWithSlug.Unit, orgWithSlug.Slug))
}

func (h *Handler) GetAllOrganizations(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAllOrganizations")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	organizationsWithSlug, err := h.store.GetAllOrganizations(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get all organizations: %w", err), logger)
		return
	}

	orgResponses := make([]OrganizationResponse, 0, len(organizationsWithSlug))
	for _, org := range organizationsWithSlug {
		orgResponses = append(orgResponses, convertOrgResponse(org.Unit, org.Slug))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, orgResponses)
}

func (h *Handler) ListOrganizationsOfCurrentUser(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListOrganizationsOfCurrentUser")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	organizationsOfUser, err := h.store.ListOrganizationsOfUser(traceCtx, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get all organizations of user: %w", err), logger)
		return
	}

	orgResponses := make([]OrganizationResponse, 0, len(organizationsOfUser))
	for _, org := range organizationsOfUser {
		orgResponses = append(orgResponses, convertOrgResponse(org.Unit, org.Slug))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, orgResponses)
}

func (h *Handler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateUnit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req Request
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), logger)
		return
	}

	updatedUnit, err := h.store.UpdateUnit(traceCtx, id, req.Name, req.Description, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update unit: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertUnitResponse(updatedUnit))
}

func (h *Handler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateOrg")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req OrgRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), logger)
		return
	}

	metadataBytes, err := json.Marshal(req.Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal metadata: %w", err), logger)
		return
	}

	// TODO: Slug Validator

	updatedOrg, err := h.store.UpdateOrg(traceCtx, slug, req.Slug, req.Name, req.Description, req.DbStrategy, metadataBytes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to update organization: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, convertOrgResponse(updatedOrg, req.Slug))
}

func (h *Handler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteOrg")
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

	err = h.store.Delete(traceCtx, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to delete unit: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

// DeleteUnit deletes a unit by its ID
func (h *Handler) DeleteUnit(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteUnit")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to delete unit: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) AddParentChild(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddParent")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var req ParentChildRequest
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	pc, err := h.store.AddParent(traceCtx, req.ParentID, req.ChildID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add parent-child relationship: %w", err), logger)
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

	subUnits, err := h.store.ListSubUnits(traceCtx, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), logger)
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
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}
	subUnits, err := h.store.ListSubUnits(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), logger)
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

	subUnits, err := h.store.ListSubUnitIDs(traceCtx, orgID, TypeOrg)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, subUnits)
}

func (h *Handler) ListUnitSubUnitIDs(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListUnitSubUnits")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	subUnits, err := h.store.ListSubUnitIDs(traceCtx, id, TypeUnit)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list sub-units: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, subUnits)
}

func (h *Handler) AddOrgMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddOrgMember")
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

	// Get email from request body
	var params struct {
		Email string `json:"email" validate:"required,email"`
	}
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &params); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	if orgID == uuid.Nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("org ID cannot be empty"), logger)
		return
	}

	members, err := h.store.AddMember(traceCtx, TypeOrg, orgID, params.Email)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add org member: %w", err), logger)
		return
	}

	orgMemberResponse := OrgMemberResponse{
		OrgID:      orgID,
		SimpleUser: h.createProfileResponseWithEmails(traceCtx, logger, members.MemberID, members.Name.String, members.Username.String, members.AvatarUrl.String),
	}
	handlerutil.WriteJSONResponse(w, http.StatusCreated, orgMemberResponse)
}

func (h *Handler) AddUnitMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "AddUnitMember")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), logger)
		return
	}

	var params struct {
		Email string `json:"email" validate:"required,email"`
	}
	if err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &params); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	member, err := h.store.AddMember(traceCtx, TypeUnit, id, params.Email)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to add unit member: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, UnitMemberResponse{
		UnitID:     id,
		SimpleUser: h.createProfileResponseWithEmails(traceCtx, logger, member.MemberID, member.Name.String, member.Username.String, member.AvatarUrl.String),
	})
}

func (h *Handler) ListOrgMembers(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListOrgMembers")
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

	// Todo: Need to recursively obtain members of the entire organization
	members, err := h.store.ListMembers(traceCtx, orgID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list org members: %w", err), logger)
		return
	}

	response := make([]user.ProfileResponse, 0, len(members))
	for _, member := range members {
		response = append(response, user.ProfileResponse(member))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) ListUnitMembers(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListUnitMembers")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), logger)
		return
	}

	members, err := h.store.ListMembers(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to list unit members: %w", err), logger)
		return
	}

	response := make([]user.ProfileResponse, 0, len(members))
	for _, memberProfile := range members {
		response = append(response, user.ProfileResponse(memberProfile))
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

func (h *Handler) RemoveOrgMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RemoveOrgMember")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	slug, err := internal.GetSlugFromContext(traceCtx)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org slug from context: %w", err), logger)
		return
	}

	_, orgID, err := h.tenantStore.GetSlugStatus(traceCtx, slug)
	if err != nil || orgID == uuid.Nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get org ID by slug: %w", err), logger)
		return
	}

	mIDStr := r.PathValue("member_id")

	if mIDStr == "" {
		http.Error(w, "member ID not provided", http.StatusBadRequest)
		return
	}
	mID, err := uuid.Parse(mIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid member ID: %w", err), logger)
		return
	}

	err = h.store.RemoveMember(traceCtx, TypeOrg, orgID, mID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to remove org member: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) RemoveUnitMember(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "RemoveUnitMember")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), logger)
		return
	}

	mIDStr := r.PathValue("member_id")
	if mIDStr == "" {
		http.Error(w, "member ID not provided", http.StatusBadRequest)
		return
	}

	mID, err := uuid.Parse(mIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid member ID: %w", err), logger)
		return
	}

	err = h.store.RemoveMember(traceCtx, TypeUnit, id, mID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to remove unit member: %w", err), logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

func (h *Handler) ListFormsOfCurrentUser(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListFormsOfCurrentUser")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	organizations, err := h.store.ListOrganizationsOfUser(traceCtx, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get all organizations of user: %w", err), logger)
		return
	}

	unitIDs := make([]uuid.UUID, 0)
	for _, org := range organizations {
		unitIDs = append(unitIDs, org.Unit.ID)
	}

	userForms, err := h.formStore.ListFormsOfUser(traceCtx, unitIDs, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get forms of user: %w", err), logger)
		return
	}

	userFormsResponse := make([]UserFormResponse, 0, len(userForms))
	for _, userForm := range userForms {
		var deadline *time.Time
		if userForm.Deadline.Valid {
			deadline = &userForm.Deadline.Time
		}

		userFormsResponse = append(userFormsResponse, UserFormResponse{
			FormID:   userForm.FormID.String(),
			Title:    userForm.Title,
			Deadline: deadline,
			Status:   userForm.Status,
		})
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, userFormsResponse)
}

func (h *Handler) UpdateUnitMemberRole(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateUnitMemberRole")
	defer span.End()

	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("id")
	if idStr == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("unit ID not provided"), logger)
		return
	}

	unitID, err := uuid.Parse(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid unit ID: %w", err), logger)
		return
	}

	mIDStr := r.PathValue("member_id")
	if mIDStr == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("member ID not provided"), logger)
		return
	}

	memberID, err := uuid.Parse(mIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid member ID: %w", err), logger)
		return
	}

	var req struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid request body: %w", err), logger)
		return
	}

	if req.Role == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("role is required"), logger)
		return
	}

	newRole := UnitRole(req.Role)

	switch newRole {
	case UnitRoleAdmin, UnitRoleMember:
		// valid role
	default:
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid role value"), logger)
		return
	}

	err = h.store.UpdateUnitMemberRole(traceCtx, unitID, memberID, newRole)
	if err != nil {
		h.problemWriter.WriteError(
			traceCtx,
			w,
			fmt.Errorf("failed to update unit member role: %w", err),
			logger,
		)
		return
	}

	logger.Info("Updated unit member role",
		zap.String("unit_id", unitID.String()),
		zap.String("member_id", memberID.String()),
		zap.String("new_role", string(newRole)),
	)

	type UpdateUnitMemberRoleResponse struct {
		UnitID   uuid.UUID `json:"unit_id"`
		MemberID uuid.UUID `json:"member_id"`
		Role     UnitRole  `json:"role"`
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, UpdateUnitMemberRoleResponse{
		UnitID:   unitID,
		MemberID: memberID,
		Role:     newRole,
	})

}
