package highlight

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

type mockHighlightQuerier struct {
	mock.Mock
}

func (m *mockHighlightQuerier) GetByFormID(ctx context.Context, formID uuid.UUID) (FormHighlight, error) {
	args := m.Called(ctx, formID)
	return args.Get(0).(FormHighlight), args.Error(1)
}

func (m *mockHighlightQuerier) UpsertByFormID(ctx context.Context, arg UpsertByFormIDParams) (FormHighlight, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(FormHighlight), args.Error(1)
}

func (m *mockHighlightQuerier) DeleteByFormID(ctx context.Context, formID uuid.UUID) (int64, error) {
	args := m.Called(ctx, formID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockHighlightQuerier) UpdateDisplayTitleByFormID(ctx context.Context, arg UpdateDisplayTitleByFormIDParams) (FormHighlight, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(FormHighlight), args.Error(1)
}

func (m *mockHighlightQuerier) GetQuestionByFormIDAndQuestionID(ctx context.Context, arg GetQuestionByFormIDAndQuestionIDParams) (GetQuestionByFormIDAndQuestionIDRow, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(GetQuestionByFormIDAndQuestionIDRow), args.Error(1)
}

func (m *mockHighlightQuerier) ListAnswerValuesByQuestionID(ctx context.Context, questionID uuid.UUID) ([][]byte, error) {
	args := m.Called(ctx, questionID)
	return args.Get(0).([][]byte), args.Error(1)
}

type mockFormExistsStore struct {
	mock.Mock
}

func (m *mockFormExistsStore) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func TestService_Clear(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		setup    func(t *testing.T, formID uuid.UUID, queries *mockHighlightQuerier, formStore *mockFormExistsStore)
		validate func(t *testing.T, err error)
	}{
		{
			name: "returns not found when highlight does not exist",
			setup: func(t *testing.T, formID uuid.UUID, queries *mockHighlightQuerier, formStore *mockFormExistsStore) {
				t.Helper()
				formStore.On("Exists", mock.Anything, formID).Return(true, nil).Once()
				queries.On("DeleteByFormID", mock.Anything, formID).Return(int64(0), nil).Once()
			},
			validate: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, internal.ErrHighlightNotFound)
			},
		},
		{
			name: "deletes highlight when it exists",
			setup: func(t *testing.T, formID uuid.UUID, queries *mockHighlightQuerier, formStore *mockFormExistsStore) {
				t.Helper()
				formStore.On("Exists", mock.Anything, formID).Return(true, nil).Once()
				queries.On("DeleteByFormID", mock.Anything, formID).Return(int64(1), nil).Once()
			},
			validate: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			formID := uuid.New()
			queries := new(mockHighlightQuerier)
			formStore := new(mockFormExistsStore)
			tc.setup(t, formID, queries, formStore)

			svc := &Service{
				logger:    zap.NewNop(),
				queries:   queries,
				formStore: formStore,
				tracer:    noop.NewTracerProvider().Tracer("test"),
			}

			err := svc.Clear(context.Background(), formID)
			if tc.validate != nil {
				tc.validate(t, err)
			}

			formStore.AssertExpectations(t)
			queries.AssertExpectations(t)
		})
	}
}
