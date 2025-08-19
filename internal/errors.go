package internal

import (
	"errors"

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
	ErrMissingAuthHeader       = errors.New("missing access token cookie")
	ErrInvalidAuthHeaderFormat = errors.New("invalid access token cookie")
	ErrInvalidJWTToken         = errors.New("invalid JWT token")
	ErrInvalidAuthUser         = errors.New("invalid authenticated user")

	// User Errors
	ErrUserNotFound    = errors.New("user not found")
	ErrNoUserInContext = errors.New("no user found in request context")

	// Unit Errors
	ErrOrgSlugNotFound      = errors.New("org slug not found")
	ErrOrgSlugAlreadyExists = errors.New("org slug already exists")
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
		return problem.NewUnauthorizedProblem("missing access token cookie")
	case errors.Is(err, ErrInvalidAuthHeaderFormat):
		return problem.NewUnauthorizedProblem("invalid access token cookie")
	case errors.Is(err, ErrInvalidJWTToken):
		return problem.NewUnauthorizedProblem("invalid JWT token")
	case errors.Is(err, ErrInvalidAuthUser):
		return problem.NewUnauthorizedProblem("invalid authenticated user")
	// User Errors
	case errors.Is(err, ErrUserNotFound):
		return problem.NewNotFoundProblem("user not found")
	case errors.Is(err, ErrNoUserInContext):
		return problem.NewUnauthorizedProblem("no user found in request context")

	// Unit Errors
	case errors.Is(err, ErrOrgSlugNotFound):
		return problem.NewNotFoundProblem("org slug not found")
	case errors.Is(err, ErrOrgSlugAlreadyExists):
		return problem.NewValidateProblem("org slug already exists")
	}
	return problem.Problem{}
}
