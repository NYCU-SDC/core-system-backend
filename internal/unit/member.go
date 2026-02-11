package unit

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal/user"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AddMember adds a member to an organization or a unit
func (s *Service) AddMember(ctx context.Context, unitType Type, id uuid.UUID, memberEmail string) (AddMemberRow, error) {
	traceCtx, span := s.tracer.Start(ctx, fmt.Sprintf("Add%sMember", unitType.String()))
	defer span.End()

	logger := logutil.WithContext(traceCtx, s.logger)

	adminCount, err := s.queries.CountMembersByRole(
		traceCtx,
		CountMembersByRoleParams{
			UnitID: id,
			Role:   UnitRoleAdmin,
		},
	)

	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "count admins")
		span.RecordError(err)
		return AddMemberRow{}, err
	}

	memberRow, err := s.queries.AddMember(traceCtx, AddMemberParams{
		UnitID:      id,
		MemberEmail: memberEmail,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "add member relationship")
		span.RecordError(err)
		return AddMemberRow{}, err
	}

	if adminCount == 0 {
		err = s.queries.UpdateMemberRole(traceCtx, UpdateMemberRoleParams{
			UnitID:   id,
			MemberID: memberRow.MemberID,
			Role:     UnitRoleAdmin,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "set first member as admin")
			span.RecordError(err)
			return AddMemberRow{}, err
		}

		memberRow.Role = UnitRoleAdmin
		logger.Info("First member added, set as admin",
			zap.String("unit_id", id.String()),
			zap.String("member_id", memberRow.MemberID.String()))
	}

	logger.Info(fmt.Sprintf("Added %s member", unitType.String()),
		zap.String("unit_id", memberRow.UnitID.String()),
		zap.String("member_id", memberRow.MemberID.String()),
		zap.String("role", string(memberRow.Role)))

	return memberRow, nil
}

// ListMembers lists all members of an organization or a unit with their emails
// This method returns []user.Profile to implement the Store interface
func (s *Service) ListMembers(ctx context.Context, id uuid.UUID) ([]user.Profile, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListMembers")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	members, err := s.queries.ListMembers(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list members")
		span.RecordError(err)
		return nil, err
	}

	if members == nil {
		members = []ListMembersRow{}
	}

	profiles := make([]user.Profile, 0, len(members))
	for _, member := range members {
		profiles = append(profiles, user.Profile{
			ID:        member.MemberID,
			Name:      member.Name.String,
			Username:  member.Username.String,
			AvatarURL: member.AvatarUrl.String,
			Emails:    user.ConvertEmailsToSlice(member.Emails),
		})
	}

	logger.Info("Listed unit members",
		zap.String("id", id.String()),
		zap.Int("count", len(profiles)),
	)

	return profiles, nil
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
