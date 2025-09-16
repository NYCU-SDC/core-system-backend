package unit

import (
	"fmt"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type SimpleUser struct {
	ID        uuid.UUID
	Name      string
	Username  string
	AvatarURL string
}

// AddMember adds a member to an organization or a unit
func (s *Service) AddMember(ctx context.Context, unitType Type, id uuid.UUID, memberID uuid.UUID) (GenericMember, error) {
	traceCtx, span := s.tracer.Start(ctx, fmt.Sprintf("Add%sMember", unitType.String()))
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	switch unitType {
	case TypeOrg:
		orgMember, err := s.queries.AddOrgMember(traceCtx, AddOrgMemberParams{
			OrgID:    id,
			MemberID: memberID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "add org member relationship")
			span.RecordError(err)
			return OrgMemberWrapper{}, err
		}

		logger.Info("Added organization member",
			zap.String("org_id", orgMember.OrgID.String()),
			zap.String("member_id", orgMember.MemberID.String()))

		return OrgMemberWrapper{orgMember}, nil

	case TypeUnit:
		unitMember, err := s.queries.AddUnitMember(traceCtx, AddUnitMemberParams{
			UnitID:   id,
			MemberID: memberID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "add unit member relationship")
			span.RecordError(err)
			return MemberWrapper{}, err
		}

		logger.Info("Added unit member",
			zap.String("unit_id", unitMember.UnitID.String()),
			zap.String("member_id", unitMember.MemberID.String()))

		unit, err := s.queries.GetUnitByID(traceCtx, id)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "get unit by id when adding member to unit")
			span.RecordError(err)
			return MemberWrapper{}, err
		}

		uid, err := uuid.Parse(unit.OrgID.String())
		if err != nil {
			logger.Error("failed to parse org id when adding member to unit", zap.Error(err))
			return nil, err
		}

		_, err = s.AddMember(ctx, TypeOrg, uid, memberID)
		if err != nil {
			logger.Error("failed to add member to organization when adding to unit", zap.Error(err))
			return nil, err
		}

		return MemberWrapper{unitMember}, nil
	}

	logger.Error("invalid unit type: ", zap.String("unitType", unitType.String()))
	return MemberWrapper{}, fmt.Errorf("invalid unit type: %s", unitType)
}

// ListMembers lists all members of an organization or a unit
func (s *Service) ListMembers(ctx context.Context, unitType Type, id uuid.UUID) ([]SimpleUser, error) {
	traceCtx, span := s.tracer.Start(ctx, fmt.Sprintf("List%sMembers", unitType.String()))
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	var simpleUsers []SimpleUser
	switch unitType {
	case TypeOrg:
		members, err := s.queries.ListOrgMembers(traceCtx, id)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "list org members")
			span.RecordError(err)
			return nil, err
		}

		simpleUsers = make([]SimpleUser, len(members))
		for i, member := range members {
			simpleUsers[i] = SimpleUser{
				ID:        member.MemberID,
				Name:      member.Name.String,
				Username:  member.Username.String,
				AvatarURL: member.AvatarUrl.String,
			}
		}

	case TypeUnit:
		members, err := s.queries.ListUnitMembers(traceCtx, id)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "list org members")
			span.RecordError(err)
			return nil, err
		}

		fmt.Println(len(members))

		simpleUsers = make([]SimpleUser, len(members))
		for i, member := range members {
			simpleUsers[i] = SimpleUser{
				ID:        member.MemberID,
				Name:      member.Name.String,
				Username:  member.Username.String,
				AvatarURL: member.AvatarUrl.String,
			}
		}
	}

	logger.Info(fmt.Sprintf("Listed %s members", unitType.String()),
		zap.String("org_id", id.String()),
		zap.Int("count", len(simpleUsers)),
	)

	return simpleUsers, nil
}

// ListUnitsMembers lists members for multiple units at once
// Todo: need to refactor to use SimpleUser
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

	var err error
	switch unitType {
	case TypeOrg:
		err = s.queries.RemoveOrgMember(traceCtx, RemoveOrgMemberParams{
			OrgID:    id,
			MemberID: memberID,
		})
	case TypeUnit:
		err = s.queries.RemoveUnitMember(traceCtx, RemoveUnitMemberParams{
			UnitID:   id,
			MemberID: memberID,
		})
	}

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
