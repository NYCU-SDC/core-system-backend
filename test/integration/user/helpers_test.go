package user

import (
	"NYCU-SDC/core-system-backend/internal/user"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	resourceManager, _, err := integration.GetOrInitResource()
	if err != nil {
		panic(err)
	}

	_, rollback, err := resourceManager.SetupPostgres()
	if err != nil {
		panic(err)
	}

	code := m.Run()

	rollback()
	resourceManager.Cleanup()

	os.Exit(code)
}

func newUserService(t *testing.T, db dbbuilder.DBTX, logger *zap.Logger) *user.Service {
	t.Helper()
	return user.NewService(logger, db, nil, nil, nil, nil)
}

func setupEmailOnlyAccountFindOrCreate(t *testing.T, db dbbuilder.DBTX) (user.FindOrCreateParams, uuid.UUID) {
	t.Helper()

	builder := userbuilder.New(t, db)
	account := builder.Create()
	email := fmt.Sprintf("emailonly-%s@example.com", uuid.NewString())
	builder.CreateEmail(account.ID, email)
	provider := "google"
	providerID := uuid.NewString()

	return user.FindOrCreateParams{
		Name:            "Test User",
		Email:           email,
		Role:            []string{"user"},
		OAuthProvider:   provider,
		OAuthProviderID: providerID,
	}, account.ID
}

func setupAuthLinkScenario(t *testing.T, db dbbuilder.DBTX) (
	ownerID uuid.UUID,
	email string,
	existingProvider string,
	existingProviderID string,
	newProvider string,
	newProviderID string,
) {
	t.Helper()

	builder := userbuilder.New(t, db)
	owner := builder.Create()
	email = fmt.Sprintf("auth-link-%s@example.com", uuid.NewString())
	existingProvider = "github"
	existingProviderID = uuid.NewString()
	builder.CreateAuth(owner.ID, email, existingProvider, existingProviderID)
	newProvider = "google"
	newProviderID = uuid.NewString()

	return owner.ID, email, existingProvider, existingProviderID, newProvider, newProviderID
}

func setupEmailOnlyCreateAuthScenario(t *testing.T, db dbbuilder.DBTX) (
	ownerID uuid.UUID,
	email string,
	provider string,
	providerID string,
) {
	t.Helper()

	builder := userbuilder.New(t, db)
	owner := builder.Create()
	email = fmt.Sprintf("setup-email-only-%s@example.com", uuid.NewString())
	builder.CreateEmail(owner.ID, email)
	provider = "google"
	providerID = uuid.NewString()

	return owner.ID, email, provider, providerID
}
