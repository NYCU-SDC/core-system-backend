package response

import (
	"context"
	"fmt"
	"testing"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/user"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type stubUserStore struct {
	getFn func(context.Context, uuid.UUID) (user.UserDetail, error)
}

func (s stubUserStore) Get(ctx context.Context, id uuid.UUID) (user.UserDetail, error) {
	return s.getFn(ctx, id)
}

func TestListBySubmittedBy_missingUser(t *testing.T) {
	t.Parallel()

	svc := Service{
		logger: zap.NewNop(),
		tracer: otel.Tracer("response/service_test"),
		userStore: stubUserStore{
			getFn: func(context.Context, uuid.UUID) (user.UserDetail, error) {
				return user.UserDetail{}, fmt.Errorf("%w", handlerutil.ErrNotFound)
			},
		},
	}

	_, err := svc.ListBySubmittedBy(context.Background(), uuid.New())
	require.ErrorIs(t, err, internal.ErrUserNotFound)
}

func TestListByFormIDAndSubmittedBy_missingUser(t *testing.T) {
	t.Parallel()

	svc := Service{
		logger: zap.NewNop(),
		tracer: otel.Tracer("response/service_test"),
		userStore: stubUserStore{
			getFn: func(context.Context, uuid.UUID) (user.UserDetail, error) {
				return user.UserDetail{}, fmt.Errorf("%w", handlerutil.ErrNotFound)
			},
		},
	}

	_, err := svc.ListByFormIDAndSubmittedBy(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, internal.ErrUserNotFound)
}
