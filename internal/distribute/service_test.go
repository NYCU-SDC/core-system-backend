package distribute_test

import (
	"NYCU-SDC/core-system-backend/internal/distribute"
	"NYCU-SDC/core-system-backend/internal/unit"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeUnitStore implements UnitStore for testing purpose
type fakeUnitStore struct {
	orgUsers  map[uuid.UUID][]uuid.UUID
	unitUsers map[uuid.UUID][]uuid.UUID
}

func (f *fakeUnitStore) ListMembers(ctx context.Context, unitType unit.Type, id uuid.UUID) ([]unit.SimpleUser, error) {
	fmt.Printf("ListMembers called with unitType: %v, id: %v\n", unitType, id)
	switch unitType {
	case unit.TypeOrg:
		if users, ok := f.orgUsers[id]; ok {
			simpleUsers := make([]unit.SimpleUser, len(users))
			for _, u := range users {
				simpleUsers = append(simpleUsers, unit.SimpleUser{ID: u})
			}
		}
		return []unit.SimpleUser{}, nil
	case unit.TypeUnit:
		if users, ok := f.unitUsers[id]; ok {
			simpleUsers := make([]unit.SimpleUser, len(users))
			for _, u := range users {
				simpleUsers = append(simpleUsers, unit.SimpleUser{ID: u})
			}
		}
		return []unit.SimpleUser{}, nil
	default:
		return nil, fmt.Errorf("invalid unit type: %v", unitType)
	}
}

func (f *fakeUnitStore) ListUnitsMembers(ctx context.Context, unitIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	result := make(map[uuid.UUID][]uuid.UUID)
	for _, id := range unitIDs {
		if users, ok := f.unitUsers[id]; ok {
			result[id] = users
		} else {
			result[id] = []uuid.UUID{}
		}
	}
	return result, nil
}

func TestService_GetRecipients(t *testing.T) {
	u1 := uuid.New()
	u2 := uuid.New()
	u3 := uuid.New()

	unitKey1 := uuid.New()
	unitKey2 := uuid.New()

	tests := []struct {
		name        string
		unitIDs     []uuid.UUID
		store       *fakeUnitStore
		expect      []uuid.UUID
		expectCount int
	}{
		{
			name:    "Should return empty when no recipients at all",
			unitIDs: []uuid.UUID{uuid.New()},
			store: &fakeUnitStore{
				unitUsers: map[uuid.UUID][]uuid.UUID{},
			},
			expect:      []uuid.UUID{},
			expectCount: 0,
		},
		{
			name:    "Should deduplicate recipients across unit",
			unitIDs: []uuid.UUID{unitKey1, unitKey2},
			store: &fakeUnitStore{
				unitUsers: map[uuid.UUID][]uuid.UUID{
					unitKey1: {u1, u2},
					unitKey2: {u2, u3},
				},
			},
			expect:      []uuid.UUID{u1, u2, u3},
			expectCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			svc := distribute.NewService(logger, tt.store)

			got, err := svc.GetRecipients(context.Background(), tt.unitIDs)
			require.NoError(t, err)

			// check total number of recipients
			require.Equal(t, tt.expectCount, len(got),
				"expected %d recipients %v, but got %d, %v", tt.expectCount, tt.expect, len(got), got)

			// check that all expected recipients are included in result
			for _, exp := range tt.expect {
				require.Contains(t, got, exp)
			}
		})
	}
}
