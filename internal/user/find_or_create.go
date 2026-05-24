package user

import (
	"context"
	"errors"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type oauthEmailOutcome int

const (
	// emailNotFound means no user owns the email yet.
	emailNotFound oauthEmailOutcome = iota
	// linkedEmailUser means an email-only account exists and OAuth auth was linked or already present.
	linkedEmailUser
	// bindingRequired means the email belongs to a different OAuth provider; caller must confirm binding.
	bindingRequired
)

// GetByOAuthProvider returns the user ID for an existing (provider, providerID) auth row.
func (s *Service) GetByOAuthProvider(ctx context.Context, provider, providerID string) (uuid.UUID, bool, error) {
	userID, err := s.queries.GetByAuth(ctx, GetByAuthParams{
		Provider:   provider,
		ProviderID: providerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, false, nil
	}
	if err != nil {
		return uuid.UUID{}, false, err
	}

	return userID, true, nil
}

// bindingResult builds a FindOrCreateResult that triggers account-binding confirmation.
func (row GetWithEarliestProviderByEmailRow) bindingResult() FindOrCreateResult {
	return FindOrCreateResult{
		UserID:             row.ID,
		ExistingName:       row.Name.String,
		ExistingProvider:   row.Provider.String,
		ExistingProviderID: row.ProviderID.String,
	}
}

// resolveOAuthByEmail locks the email row and either links OAuth to an email-only account,
// signals that binding confirmation is required, or reports that no user owns the email.
func (s *Service) resolveOAuthByEmail(
	ctx context.Context,
	params FindOrCreateParams,
) (oauthEmailOutcome, FindOrCreateResult, error) {
	logger := logutil.WithContext(ctx, s.logger)

	var outcome oauthEmailOutcome
	var result FindOrCreateResult

	err := s.withTransaction(ctx, func(qtx *Queries) error {
		emailRow, err := qtx.GetEmailForUpdate(ctx, params.Email)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = emailNotFound
			return nil
		}
		if err != nil {
			return err
		}

		existingUser, err := qtx.GetWithEarliestProviderByEmail(ctx, params.Email)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				outcome = emailNotFound
				return nil
			}
			return err
		}

		if existingUser.Provider.Valid {
			if existingUser.Provider.String == params.OAuthProvider &&
				existingUser.ProviderID.String == params.OAuthProviderID {
				outcome = linkedEmailUser
				result = FindOrCreateResult{UserID: existingUser.ID}
				return nil
			}

			logger.Info("Email already exists under different provider, binding confirmation required",
				zap.String("name", existingUser.Name.String),
				zap.String("email", params.Email),
				zap.String("existing_provider", existingUser.Provider.String),
				zap.String("new_provider", params.OAuthProvider),
			)

			outcome = bindingRequired
			result = existingUser.bindingResult()

			return nil
		}

		logger.Info("User has been initialized",
			zap.String("name", params.Name),
			zap.String("email", params.Email))

		_, err = qtx.CreateAuth(ctx, CreateAuthParams{
			UserID:      existingUser.ID,
			UserEmailID: userEmailIDParam(emailRow.ID),
			Provider:    params.OAuthProvider,
			ProviderID:  params.OAuthProviderID,
		})
		if err != nil {
			return databaseutil.WrapDBError(err, logger, "create auth for email-only user")
		}

		outcome = linkedEmailUser
		result = FindOrCreateResult{UserID: existingUser.ID}

		return nil
	})
	if err != nil {
		wrapped := databaseutil.WrapDBError(err, logger, "check user existence by email")
		if errors.Is(wrapped, databaseutil.ErrUniqueViolation) {
			id, lookupErr := s.queries.GetByAuth(ctx, GetByAuthParams{
				Provider:   params.OAuthProvider,
				ProviderID: params.OAuthProviderID,
			})
			if lookupErr == nil {
				logger.Debug("Concurrent OAuth link won race, returning existing auth owner",
					zap.String("email", params.Email),
					zap.String("provider", params.OAuthProvider),
					zap.String("user_id", id.String()))
				return linkedEmailUser, FindOrCreateResult{UserID: id}, nil
			}
		}

		return 0, FindOrCreateResult{}, wrapped
	}

	return outcome, result, nil
}

// bindingResultIfEmailHasAuth returns binding details when the email is already tied to another OAuth provider.
func (s *Service) bindingResultIfEmailHasAuth(ctx context.Context, email string) (FindOrCreateResult, bool, error) {
	row, err := s.queries.GetWithEarliestProviderByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FindOrCreateResult{}, false, nil
		}

		return FindOrCreateResult{}, false, err
	}
	if !row.Provider.Valid {
		return FindOrCreateResult{}, false, nil
	}

	return row.bindingResult(), true, nil
}

type oauthCreateConflictKind int

const (
	oauthCreateConflictNone oauthCreateConflictKind = iota
	oauthCreateConflictViaAuth
	oauthCreateConflictViaEmail
)

type oauthCreateConflictRecovery struct {
	kind      oauthCreateConflictKind
	accountID uuid.UUID
}

// recoverOAuthCreateConflict recovers the canonical account ID after a concurrent OAuth signup
// hits a unique or email-ownership conflict.
func (s *Service) recoverOAuthCreateConflict(
	ctx context.Context,
	err error,
	oauthProvider, oauthProviderID, email string,
) (oauthCreateConflictRecovery, bool) {
	if !errors.Is(err, databaseutil.ErrUniqueViolation) && !errors.Is(err, internal.ErrEmailConflict) {
		return oauthCreateConflictRecovery{}, false
	}

	recoveredID, lookupErr := s.queries.GetByAuth(ctx, GetByAuthParams{
		Provider:   oauthProvider,
		ProviderID: oauthProviderID,
	})
	if lookupErr == nil {
		logger := logutil.WithContext(ctx, s.logger)
		logger.Debug("Recovered OAuth create conflict via auth lookup",
			zap.String("provider", oauthProvider),
			zap.String("provider_id", oauthProviderID),
			zap.String("user_id", recoveredID.String()))
		return oauthCreateConflictRecovery{
			kind:      oauthCreateConflictViaAuth,
			accountID: recoveredID,
		}, true
	}

	if email != "" {
		recoveredID, lookupErr := s.queries.GetByEmail(ctx, email)
		if lookupErr == nil {
			logger := logutil.WithContext(ctx, s.logger)
			logger.Debug("Recovered OAuth create conflict via email lookup",
				zap.String("email", email),
				zap.String("provider", oauthProvider),
				zap.String("user_id", recoveredID.String()))

			return oauthCreateConflictRecovery{
				kind:      oauthCreateConflictViaEmail,
				accountID: recoveredID,
			}, true
		}
	}

	return oauthCreateConflictRecovery{}, false
}

// recoverOAuthAccountAfterCreateError handles concurrent signup races after createOAuth fails.
func (s *Service) recoverOAuthAccountAfterCreateError(
	ctx context.Context,
	createErr error,
	params FindOrCreateParams,
) (FindOrCreateResult, error) {
	recovery, ok := s.recoverOAuthCreateConflict(ctx, createErr, params.OAuthProvider, params.OAuthProviderID, params.Email)
	if !ok {
		return FindOrCreateResult{}, createErr
	}

	switch recovery.kind {
	case oauthCreateConflictViaAuth:
		logger := logutil.WithContext(ctx, s.logger)
		logger.Info("Recovered OAuth user after concurrent signup",
			zap.String("recovery", "via_auth"),
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", recovery.accountID.String()))
		return FindOrCreateResult{UserID: recovery.accountID}, nil
	case oauthCreateConflictViaEmail:
		logger := logutil.WithContext(ctx, s.logger)
		logger.Info("Recovered OAuth user after concurrent signup, resolving email conflict",
			zap.String("recovery", "via_email"),
			zap.String("email", params.Email),
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", recovery.accountID.String()))
		return s.resolveOAuthEmailConflict(ctx, params, recovery.accountID)
	default:
		return FindOrCreateResult{}, createErr
	}
}

// createWithAuth creates a new user with OAuth auth. The bool return is true when a brand-new account was created.
// On concurrent signup conflicts it attempts recovery via recoverOAuthAccountAfterCreateError.
func (s *Service) createWithAuth(
	ctx context.Context,
	params FindOrCreateParams,
) (FindOrCreateResult, bool, error) {
	logger := logutil.WithContext(ctx, s.logger)

	finalRoles := buildGlobalRoleSet(params.Role, params.Email)
	logger.Info("Final roles for new user", zap.Strings("roles", finalRoles))

	placeholderAvatar := resolveAvatarURL(params.Name, "")

	newUserID, err := s.createOAuth(ctx, createOAuthParams{
		Name:              params.Name,
		AvatarPlaceholder: placeholderAvatar,
		Roles:             finalRoles,
		Email:             params.Email,
		OAuthProvider:     params.OAuthProvider,
		OAuthProviderID:   params.OAuthProviderID,
	})
	if err != nil {
		recovered, recoverErr := s.recoverOAuthAccountAfterCreateError(ctx, err, params)
		if recoverErr != nil {
			return FindOrCreateResult{}, false, recoverErr
		}
		logger.Info("OAuth user create failed but concurrent signup was recovered",
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", recovered.UserID.String()))
		return recovered, false, nil
	}

	return FindOrCreateResult{UserID: newUserID}, true, nil
}

// resolveOAuthEmailConflict re-evaluates email state when create hits a unique or ownership conflict.
// Outcomes: (1) link to email-only account, (2) binding confirmation for cross-provider email,
// (3) return recoveredAccountID when no email conflict remains.
// bindingResultIfEmailHasAuth is a non-locking second pass when resolveOAuthByEmail reports emailNotFound
// but the create race may have left the email tied to another provider.
func (s *Service) resolveOAuthEmailConflict(
	ctx context.Context,
	params FindOrCreateParams,
	recoveredAccountID uuid.UUID,
) (FindOrCreateResult, error) {
	logger := logutil.WithContext(ctx, s.logger)

	outcome, result, err := s.resolveOAuthByEmail(ctx, params)
	if err != nil {
		return FindOrCreateResult{}, err
	}
	switch outcome {
	case bindingRequired:
		logger.Debug("Resolved OAuth email conflict via binding confirmation",
			zap.String("email", params.Email),
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", result.UserID.String()))
		return result, nil
	case linkedEmailUser:
		logger.Debug("Resolved OAuth email conflict by linking to existing account",
			zap.String("email", params.Email),
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", result.UserID.String()))
		return result, nil
	case emailNotFound:
		// Create lost the race but email row may still exist with another provider.
		binding, ok, err := s.bindingResultIfEmailHasAuth(ctx, params.Email)
		if err != nil {
			return FindOrCreateResult{}, err
		} else if ok {
			logger.Debug("Resolved OAuth email conflict via non-locking binding check",
				zap.String("email", params.Email),
				zap.String("provider", params.OAuthProvider),
				zap.String("user_id", binding.UserID.String()))
			return binding, nil
		}

		// No conflicting email state — return the account recovered from the error path.
		logger.Debug("Resolved OAuth email conflict with recovered account",
			zap.String("email", params.Email),
			zap.String("provider", params.OAuthProvider),
			zap.String("user_id", recoveredAccountID.String()))
		return FindOrCreateResult{UserID: recoveredAccountID}, nil
	}

	logger.Debug("Resolved OAuth email conflict with recovered account (fallback)",
		zap.String("email", params.Email),
		zap.String("provider", params.OAuthProvider),
		zap.String("user_id", recoveredAccountID.String()))
	return FindOrCreateResult{UserID: recoveredAccountID}, nil
}

// finishSignup runs post-commit work for a newly created OAuth user: avatar download and default org membership.
// Failures are logged but do not fail the login flow.
func (s *Service) finishSignup(ctx context.Context, userID uuid.UUID, remoteAvatar, email string) {
	logger := logutil.WithContext(ctx, s.logger)

	if remoteAvatar != "" && s.fileOperator != nil {
		storedAvatar := s.downloadAndSaveAvatar(ctx, remoteAvatar, userID)
		if storedAvatar != "" {
			userInfo, getErr := s.queries.Get(ctx, userID)
			if getErr != nil {
				logger.Warn("Failed to load user for avatar update", zap.String("user_id", userID.String()), zap.Error(getErr))
			} else {
				_, err := s.queries.Update(ctx, UpdateParams{
					ID:       userID,
					Name:     userInfo.Name,
					Username: userInfo.Username,
					AvatarUrl: pgtype.Text{
						String: storedAvatar,
						Valid:  true,
					},
					IsOnboarded: userInfo.IsOnboarded,
				})
				if err != nil {
					logger.Warn("Failed to update user avatar URL after download",
						zap.String("user_id", userID.String()),
						zap.Error(err))
				}
			}
		}
	}

	defaultOrgRole, ok := DefaultOrgRole(email)
	if ok && s.orgWriter != nil && s.orgResolver != nil {
		const defaultOrgSlug = "SDC"
		defaultOrgID, resolveErr := s.orgResolver.GetOrgIDBySlug(ctx, defaultOrgSlug)
		if resolveErr != nil {
			logger.Warn("failed to resolve default org slug",
				zap.String("slug", defaultOrgSlug),
				zap.Error(resolveErr))
		} else {
			err := s.orgWriter.AddMemberWithRole(ctx, defaultOrgID, userID, defaultOrgRole)
			if err != nil {
				logger.Warn("failed to apply default org role", zap.Error(err))
			}
		}
	}
}
