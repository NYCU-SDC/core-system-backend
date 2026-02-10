package internal

import (
	"errors"
	"fmt"

	"github.com/NYCU-SDC/summer/pkg/problem"
)

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
	ErrUserNotFound       = errors.New("user not found")
	ErrNoUserInContext    = errors.New("no user found in request context")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrUserOnboarded      = errors.New("user already onboarded")
	ErrUsernameConflict   = errors.New("user name already taken")
	ErrDatabaseError      = errors.New("database error")

	// OAuth Email Errors
	ErrFailedToExtractEmail = errors.New("failed to extract email from OAuth token")
	ErrFailedToCreateEmail  = errors.New("failed to create email record for OAuth user")

	// Unit Errors
	ErrOrgSlugNotFound      = errors.New("org slug not found")
	ErrOrgSlugAlreadyExists = errors.New("org slug already exists")
	ErrOrgSlugInvalid       = errors.New("org slug is invalid")
	ErrUnitNotFound         = errors.New("unit not found")
	ErrSlugNotBelongToUnit  = errors.New("slug not belong to unit")

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
	ErrValidationFailed           = errors.New("validation failed")
	ErrInvalidSourceIDWithChoices = errors.New("cannot specify both source_id and choices")
	ErrInvalidSourceIDForType     = errors.New("source_id is not supported for this question type")

	// Response Errors
	ErrResponseNotFound = errors.New("response not found")

	// Workflow Errors
	ErrWorkflowValidationFailed = errors.New("workflow validation failed")
	ErrWorkflowNotActive        = errors.New("workflow is not active")
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

	// Form Errors
	case errors.Is(err, ErrFormNotFound):
		return problem.NewNotFoundProblem("form not found")
	case errors.Is(err, ErrFormNotDraft):
		return problem.NewValidateProblem("form is not in draft status")

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
	case errors.Is(err, ErrInvalidSourceIDWithChoices):
		return problem.NewBadRequestProblem("cannot specify both source_id and choices")
	case errors.Is(err, ErrInvalidSourceIDForType):
		return problem.NewBadRequestProblem("source_id is not supported for this question type")

	// Response Errors
	case errors.Is(err, ErrResponseNotFound):
		return problem.NewNotFoundProblem("response not found")

	// Validation Errors
	case errors.Is(err, ErrValidationFailed):
		return problem.NewValidateProblem("validation failed")

	// Workflow Errors
	case errors.Is(err, ErrWorkflowValidationFailed):
		return problem.NewValidateProblem("workflow validation failed")
	case errors.Is(err, ErrWorkflowNotActive):
		return problem.NewValidateProblem("workflow is not active")
	}
	return problem.Problem{}
}
