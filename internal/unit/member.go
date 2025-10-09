package unit

import (
	"fmt"

	"NYCU-SDC/core-system-backend/internal/user"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

// AddMember adds a member to an organization or a unit
func (s *Service) AddMember(ctx context.Context, unitType Type, id uuid.UUID, memberEmail string) (AddMemberRow, error) {
	traceCtx, span := s.tracer.Start(ctx, fmt.Sprintf("Add%sMember", unitType.String()))
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)
	memberRow, err := s.queries.AddMember(traceCtx, AddMemberParams{
		UnitID:      id,
		MemberEmail: memberEmail,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "add member relationship")
		span.RecordError(err)
		return AddMemberRow{}, err
	}

	logger.Info(fmt.Sprintf("Added %s member", unitType.String()),
		zap.String("unit_id", memberRow.UnitID.String()),
		zap.String("member_id", memberRow.MemberID.String()))

	return memberRow, nil
}

// ListMembers lists all members of an organization or a unit
func (s *Service) ListMembers(ctx context.Context, id uuid.UUID) ([]user.Profile, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListMembers")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	var simpleUsers []user.Profile
	members, err := s.queries.ListMembers(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list org members")
		span.RecordError(err)
		return nil, err
	}

	simpleUsers = make([]user.Profile, len(members))
	for i, member := range members {
		simpleUsers[i] = user.Profile{
			ID:        member.MemberID,
			Name:      member.Name.String,
			Username:  member.Username.String,
			AvatarURL: member.AvatarUrl.String,
			Email:     member.Email,
		}
	}

	logger.Info("Listed unit members",
		zap.String("id", id.String()),
		zap.Int("count", len(simpleUsers)),
	)

	return simpleUsers, nil
}

// ListUnitsMembers lists members for multiple units at once
func (s *Service) ListUnitsMembers(ctx context.Context, unitIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListMultiUnitMembers")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	membersMap := make(map[uuid.UUID][]uuid.UUID)
	if len(unitIDs) == 0 {
		return membersMap, nil
	}

	rows, err := s.queries.ListUnitsMembers(traceCtx, unitIDs)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list multiple unit members")
		span.RecordError(err)
		return nil, err
	}

	for _, row := range rows {
		membersMap[row.UnitID] = append(membersMap[row.UnitID], row.MemberID)
	}

	logger.Info("Listed multiple unit members",
		zap.Int("unit_count", len(membersMap)),
		zap.String("unit_ids", fmt.Sprintf("%v", unitIDs)))

	return membersMap, nil
}

// RemoveMember removes a member from an organization or a unit
func (s *Service) RemoveMember(ctx context.Context, unitType Type, id uuid.UUID, memberID uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, fmt.Sprintf("Remove%sMember", unitType.String()))
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.RemoveMember(traceCtx, RemoveMemberParams{
		UnitID:   id,
		MemberID: memberID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, fmt.Sprintf("remove %s member", unitType.String()))
		span.RecordError(err)
		return err
	}

	logger.Info(fmt.Sprintf("Removed %s member", unitType.String()),
		zap.String("org_id", id.String()),
		zap.String("member_id", memberID.String()))

	return nil
}
