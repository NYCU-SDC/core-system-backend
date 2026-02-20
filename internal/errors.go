package internal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/google/uuid"
)

type ErrResponseNotComplete struct {
	NotCompleteSections []struct {
		Title    string
		ID       uuid.UUID
		Progress string
	}
}

func (s ErrResponseNotComplete) Error() string {
	sectionIDs := make([]string, len(s.NotCompleteSections))
	for i, section := range s.NotCompleteSections {
		sectionIDs[i] = fmt.Sprintf("Title: %s, ID: %s, Progress: %s", section.Title, section.ID.String(), section.Progress)
	}

	return "response is not complete, not complete sections: " + strings.Join(sectionIDs, "; ")
}

var (
	// Auth Errors
	ErrInvalidRefreshToken  = errors.New("invalid refresh token")
	ErrProviderNotFound     = errors.New("provider not found")
	ErrNewStateFailed       = errors.New("failed to create new jwt state")
	ErrOAuthError           = errors.New("failed to finish OAuth flow, OAuth error received")
	ErrInvalidExchangeToken = errors.New("invalid exchange token")
	ErrInvalidCallbackInfo  = errors.New("invalid callback info")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrUnauthorizedError    = errors.New("unauthorized error")
	ErrInternalServerError  = errors.New("internal server error")
	ErrForbiddenError       = errors.New("forbidden error")
	ErrNotFound             = errors.New("not found")

	// JWT Authentication Errors
	ErrMissingAuthHeader       = errors.New("missing access token")
	ErrInvalidAuthHeaderFormat = errors.New("invalid access token")
	ErrInvalidJWTToken         = errors.New("invalid JWT token")
	ErrInvalidAuthUser         = errors.New("invalid authenticated user")

	// User Errors
	ErrUserNotFound         = errors.New("user not found")
	ErrNoUserInContext      = errors.New("no user found in request context")
	ErrEmailAlreadyExists   = errors.New("email already exists")
	ErrUserOnboarded        = errors.New("user already onboarded")
	ErrUsernameConflict     = errors.New("user name already taken")
	ErrDatabaseError        = errors.New("database error")
	ErrUserNotInAllowedList = errors.New("user not in allowed onboarding list")

	// OAuth Email Errors
	ErrFailedToExtractEmail = errors.New("failed to extract email from OAuth token")
	ErrFailedToCreateEmail  = errors.New("failed to create email record for OAuth user")

	// Unit Errors
	ErrOrgSlugNotFound       = errors.New("org slug not found")
	ErrOrgSlugAlreadyExists  = errors.New("org slug already exists")
	ErrOrgSlugInvalid        = errors.New("org slug is invalid")
	ErrUnitNotFound          = errors.New("unit not found")
	ErrSlugNotBelongToUnit   = errors.New("slug not belong to unit")
	ErrInvalidEmailFormat    = errors.New("invalid email format")
	ErrMemberEmailNotFound   = errors.New("member email not found")
	ErrCannotRemoveLastAdmin = errors.New("cannot remove the last admin of the unit")

	ErrMissingUnitID      = errors.New("missing unit id")
	ErrInvalidUnitID      = errors.New("invalid unit id")
	ErrMissingMemberID    = errors.New("missing member id")
	ErrInvalidMemberID    = errors.New("invalid member id")
	ErrInvalidRequestBody = errors.New("invalid request body")
	ErrInvalidRole        = errors.New("invalid role")

	// Inbox Errors
	ErrInvalidIsReadParameter     = errors.New("invalid isRead parameter")
	ErrInvalidIsStarredParameter  = errors.New("invalid isStarred parameter")
	ErrInvalidIsArchivedParameter = errors.New("invalid isArchived parameter")
	ErrInvalidSearchParameter     = errors.New("invalid search parameter")
	ErrSearchTooLong              = errors.New("search string exceeds maximum length")

	// Form Errors
	ErrFormNotFound       = errors.New("form not found")
	ErrFormNotDraft       = fmt.Errorf("form is not in draft status")
	ErrFormDeadlinePassed = errors.New("form deadline has passed")

	// Question Errors
	ErrQuestionNotFound           = errors.New("question not found")
	ErrQuestionRequired           = errors.New("question is required but not answered")
	ErrQuestionTypeMismatch       = errors.New("question type does not match the expected type")
	ErrValidationFailed           = errors.New("validation failed")
	ErrInvalidSourceIDWithChoices = errors.New("cannot specify both source_id and choices")
	ErrInvalidSourceIDForType     = errors.New("source_id is not supported for this question type")

	// Response Errors
	ErrResponseNotFound       = errors.New("response not found")
	ErrResponseAlreadyExists  = errors.New("user already has a response for this form")
	ErrResponseFormIDMismatch = errors.New("response form ID does not match the expected form ID")

	// Workflow Errors
	ErrWorkflowValidationFailed      = errors.New("workflow validation failed")
	ErrWorkflowResolveSectionsFailed = errors.New("workflow resolve sections failed")
	ErrWorkflowNotActive             = errors.New("workflow is not active")
	ErrUnmarshalWorkflow             = errors.New("failed to unmarshal workflow")
	ErrMarshalWorkflow               = errors.New("failed to marshal workflow")
	ErrUnmarshalAPIWorkflow          = errors.New("failed to unmarshal API workflow")
	ErrUnmarshalDBWorkflow           = errors.New("failed to unmarshal database workflow")
	ErrWorkflowNodeNotFound          = errors.New("node not found in current workflow")
	ErrMarshalMergedWorkflow         = errors.New("failed to marshal merged workflow")

	// File Errors
	ErrFileNotFound       = errors.New("file not found")
	ErrFileTooLarge       = errors.New("file exceeds maximum size")
	ErrInvalidFileID      = errors.New("invalid file ID")
	ErrInvalidMultipart   = errors.New("failed to parse multipart form")
	ErrFailedToSaveFile   = errors.New("failed to save file")
	ErrFailedToDeleteFile = errors.New("failed to delete file")
	ErrInvalidLimit       = errors.New("invalid limit parameter")
	ErrInvalidOffset      = errors.New("invalid offset parameter")
	ErrInvalidFileType    = errors.New("file type is not allowed")
	ErrCoverImageTooLarge = errors.New("cover image exceeds maximum size")
	ErrInvalidImageFormat = errors.New("image format is invalid")
)

func NewProblemWriter() *problem.HttpWriter {
	return problem.NewWithMapping(ErrorHandler)
}

func ErrorHandler(err error) problem.Problem {
	switch {
	case errors.Is(err, ErrInvalidRefreshToken):
		return problem.NewNotFoundProblem("refresh token not found")
	case errors.Is(err, ErrProviderNotFound):
		return problem.NewNotFoundProblem("provider not found")
	case errors.Is(err, ErrInvalidExchangeToken):
		return problem.NewValidateProblem("invalid exchange token")
	case errors.Is(err, ErrInvalidCallbackInfo):
		return problem.NewValidateProblem("invalid callback info")
	case errors.Is(err, ErrPermissionDenied):
		return problem.NewForbiddenProblem("permission denied")
	case errors.Is(err, ErrUnauthorizedError):
		return problem.NewUnauthorizedProblem("unauthorized error")
	case errors.Is(err, ErrInternalServerError):
		return problem.NewInternalServerProblem("internal server error")
	case errors.Is(err, ErrForbiddenError):
		return problem.NewForbiddenProblem("forbidden error")
	case errors.Is(err, ErrNotFound):
		return problem.NewNotFoundProblem("not found")
	// JWT Authentication Errors
	case errors.Is(err, ErrMissingAuthHeader):
		return problem.NewUnauthorizedProblem("missing access token")
	case errors.Is(err, ErrInvalidAuthHeaderFormat):
		return problem.NewUnauthorizedProblem("invalid access token")
	case errors.Is(err, ErrInvalidJWTToken):
		return problem.NewUnauthorizedProblem("invalid JWT token")
	case errors.Is(err, ErrInvalidAuthUser):
		return problem.NewUnauthorizedProblem("invalid authenticated user")
	// User Errors
	case errors.Is(err, ErrUserNotFound):
		return problem.NewNotFoundProblem("user not found")
	case errors.Is(err, ErrNoUserInContext):
		return problem.NewUnauthorizedProblem("no user found in request context")
	case errors.Is(err, ErrEmailAlreadyExists):
		return problem.NewValidateProblem("email already exists")
	case errors.Is(err, ErrUserOnboarded):
		return problem.NewValidateProblem("user already onboarded")
	case errors.Is(err, ErrUsernameConflict):
		return problem.NewValidateProblem("username already taken")
	case errors.Is(err, ErrDatabaseError):
		return problem.NewBadRequestProblem("database error")
	case errors.Is(err, ErrUserNotInAllowedList):
		return problem.NewForbiddenProblem("user not in allowed onboarding list")

	// OAuth Email Errors
	case errors.Is(err, ErrFailedToExtractEmail):
		return problem.NewInternalServerProblem("failed to extract email from OAuth token")
	case errors.Is(err, ErrFailedToCreateEmail):
		return problem.NewInternalServerProblem("failed to create email record for OAuth user")

	// Unit Errors
	case errors.Is(err, ErrOrgSlugNotFound):
		return problem.NewNotFoundProblem("org slug not found")
	case errors.Is(err, ErrOrgSlugAlreadyExists):
		return problem.NewValidateProblem("org slug already exists")
	case errors.Is(err, ErrOrgSlugInvalid):
		return problem.NewValidateProblem("org slug is invalid")
	case errors.Is(err, ErrUnitNotFound):
		return problem.NewNotFoundProblem("unit not found")
	case errors.Is(err, ErrSlugNotBelongToUnit):
		return problem.NewNotFoundProblem("slug not belong to unit")
	case errors.Is(err, ErrInvalidEmailFormat):
		return problem.NewValidateProblem("invalid email format")
	case errors.Is(err, ErrMemberEmailNotFound):
		return problem.NewBadRequestProblem("member email not found")
	case errors.Is(err, ErrCannotRemoveLastAdmin):
		return problem.NewValidateProblem("cannot remove the last admin of the unit")
	case errors.Is(err, ErrMissingUnitID):
		return problem.NewBadRequestProblem("unit id is required")
	case errors.Is(err, ErrInvalidUnitID):
		return problem.NewBadRequestProblem("invalid unit id")
	case errors.Is(err, ErrMissingMemberID):
		return problem.NewBadRequestProblem("member id is required")
	case errors.Is(err, ErrInvalidMemberID):
		return problem.NewBadRequestProblem("invalid member id")
	case errors.Is(err, ErrInvalidRequestBody):
		return problem.NewBadRequestProblem("invalid request body")
	case errors.Is(err, ErrInvalidRole):
		return problem.NewValidateProblem("invalid role value")

	// Form Errors
	case errors.Is(err, ErrFormNotFound):
		return problem.NewNotFoundProblem("form not found")
	case errors.Is(err, ErrFormNotDraft):
		return problem.NewValidateProblem("form is not in draft status")
	case errors.Is(err, ErrCoverImageTooLarge):
		return problem.NewValidateProblem("cover image exceeds maximum size (max 2MB)")
	case errors.Is(err, ErrInvalidImageFormat):
		return problem.NewValidateProblem("image format is invalid")

	// Inbox Errors
	case errors.Is(err, ErrInvalidIsReadParameter):
		return problem.NewValidateProblem("invalid isRead parameter")
	case errors.Is(err, ErrInvalidIsStarredParameter):
		return problem.NewValidateProblem("invalid isStarred parameter")
	case errors.Is(err, ErrInvalidIsArchivedParameter):
		return problem.NewValidateProblem("invalid isArchived parameter")
	case errors.Is(err, ErrInvalidSearchParameter):
		return problem.NewValidateProblem("invalid search parameter")
	case errors.Is(err, ErrSearchTooLong):
		return problem.NewValidateProblem("search string exceeds maximum length")
	case errors.Is(err, ErrFormDeadlinePassed):
		return problem.NewValidateProblem("form deadline has passed")

	// Question Errors
	case errors.Is(err, ErrQuestionNotFound):
		return problem.NewNotFoundProblem("question not found")
	case errors.Is(err, ErrQuestionRequired):
		return problem.NewValidateProblem("question is required but not answered")
	case errors.Is(err, ErrQuestionTypeMismatch):
		return problem.NewValidateProblem("question type does not match the expected type")
	case errors.Is(err, ErrInvalidSourceIDWithChoices):
		return problem.NewBadRequestProblem("cannot specify both source_id and choices")
	case errors.Is(err, ErrInvalidSourceIDForType):
		return problem.NewBadRequestProblem("source_id is not supported for this question type")

	// Response Errors
	case errors.Is(err, ErrResponseNotFound):
		return problem.NewNotFoundProblem("response not found")
	case errors.Is(err, ErrResponseAlreadyExists):
		return problem.NewValidateProblem("user already has a response for this form")

	// Submit Errors
	case errors.Is(err, ErrResponseNotComplete{}):
		return problem.NewValidateProblem(err.Error())

	// Validation Errors
	case errors.Is(err, ErrValidationFailed):
		return problem.NewValidateProblem("validation failed")

	// Workflow Errors
	case errors.Is(err, ErrWorkflowValidationFailed):
		return problem.NewValidateProblem("workflow validation failed")
	case errors.Is(err, ErrWorkflowResolveSectionsFailed):
		return problem.NewValidateProblem("failed to resolve workflow sections")
	case errors.Is(err, ErrWorkflowNotActive):
		return problem.NewValidateProblem("workflow is not active")
	case errors.Is(err, ErrUnmarshalWorkflow):
		return problem.NewBadRequestProblem("failed to unmarshal workflow")
	case errors.Is(err, ErrMarshalWorkflow):
		return problem.NewInternalServerProblem("failed to marshal workflow")
	case errors.Is(err, ErrUnmarshalAPIWorkflow):
		return problem.NewBadRequestProblem("failed to unmarshal API workflow")
	case errors.Is(err, ErrUnmarshalDBWorkflow):
		return problem.NewInternalServerProblem("failed to unmarshal database workflow")
	case errors.Is(err, ErrWorkflowNodeNotFound):
		return problem.NewValidateProblem("node not found in current workflow, please create it first using CreateNode API")
	case errors.Is(err, ErrMarshalMergedWorkflow):
		return problem.NewInternalServerProblem("failed to marshal merged workflow")

	// File Errors
	case errors.Is(err, ErrFileNotFound):
		return problem.NewNotFoundProblem("file not found")
	case errors.Is(err, ErrFileTooLarge):
		return problem.NewValidateProblem("file exceeds maximum size (max 100MB)")
	case errors.Is(err, ErrInvalidFileID):
		return problem.NewBadRequestProblem("invalid file ID")
	case errors.Is(err, ErrInvalidMultipart):
		return problem.NewBadRequestProblem("failed to parse multipart form")
	case errors.Is(err, ErrFailedToSaveFile):
		return problem.NewInternalServerProblem("failed to save file")
	case errors.Is(err, ErrFailedToDeleteFile):
		return problem.NewInternalServerProblem("failed to delete file")
	case errors.Is(err, ErrInvalidLimit):
		return problem.NewBadRequestProblem("invalid limit parameter")
	case errors.Is(err, ErrInvalidOffset):
		return problem.NewBadRequestProblem("invalid offset parameter")
	case errors.Is(err, ErrInvalidFileType):
		return problem.NewValidateProblem("file type is not allowed")
	}
	return problem.Problem{}
}
