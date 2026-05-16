package view

import (
	"context"
	"errors"
	"fmt"

	"NYCU-SDC/core-system-backend/internal"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	defaultViewTitle = "尚未命名的分頁"
)

type Querier interface {
	Get(ctx context.Context, arg GetParams) (View, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]View, error)
	ListTitlesByFormID(ctx context.Context, formID uuid.UUID) ([]string, error)
	MaxOrder(ctx context.Context, formID uuid.UUID) (int32, error)
	Create(ctx context.Context, arg CreateParams) (View, error)
	UpdateTitle(ctx context.Context, arg UpdateTitleParams) (View, error)
	UpdateOrder(ctx context.Context, arg UpdateOrderParams) (UpdateOrderRow, error)
	UpdateLocked(ctx context.Context, arg UpdateLockedParams) (View, error)
	DeleteIfUnlocked(ctx context.Context, arg DeleteIfUnlockedParams) (DeleteIfUnlockedRow, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	FormExists(ctx context.Context, id uuid.UUID) (bool, error)
	ShiftOrders(ctx context.Context, arg ShiftOrdersParams) error
	WithTx(tx pgx.Tx) *Queries
}

type TxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

type Service struct {
	db      DBTX
	logger  *zap.Logger
	queries Querier
	tracer  trace.Tracer
}

func NewService(logger *zap.Logger, db DBTX) *Service {
	return &Service{
		db:      db,
		logger:  logger,
		queries: New(db),
		tracer:  otel.Tracer("view/service"),
	}
}

// generateDefaultTitle returns a unique default title for a new view given existing titles.
func generateDefaultTitle(existingTitles []string) string {
	titleSet := make(map[string]struct{}, len(existingTitles))
	for _, t := range existingTitles {
		titleSet[t] = struct{}{}
	}

	_, exists := titleSet[defaultViewTitle]
	if !exists {
		return defaultViewTitle
	}

	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s（%d）", defaultViewTitle, i)
		_, exists := titleSet[candidate]
		if !exists {
			return candidate
		}
	}
}

// generateDuplicateTitle returns a unique title for a duplicated view.
// It wraps the original title in another layer of "的副本" until no conflict is found.
func generateDuplicateTitle(originalTitle string, existingTitles []string) string {
	titleSet := make(map[string]struct{}, len(existingTitles))
	for _, t := range existingTitles {
		titleSet[t] = struct{}{}
	}

	candidate := "「" + originalTitle + "」的副本"
	for {
		_, exists := titleSet[candidate]
		if !exists {
			return candidate
		}
		candidate = "「" + candidate + "」的副本"
	}
}

// isUniqueViolation returns true if the pgconn error is a unique_violation (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// Create creates a new view for the given form with a generated default title.
func (s *Service) Create(ctx context.Context, formID uuid.UUID) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exist, err := s.queries.FormExists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return View{}, err
	}
	if !exist {
		return View{}, internal.ErrFormNotFound
	}

	titles, err := s.queries.ListTitlesByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list view titles")
		span.RecordError(err)
		return View{}, err
	}

	title := generateDefaultTitle(titles)

	order, err := s.queries.MaxOrder(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get max order")
		span.RecordError(err)
		return View{}, err
	}

	newView, err := s.queries.Create(traceCtx, CreateParams{
		FormID: formID,
		Title:  title,
		Order:  order + 1,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return View{}, internal.ErrViewNameDuplicate
		}
		err = databaseutil.WrapDBError(err, logger, "create view")
		span.RecordError(err)
		return View{}, err
	}

	return newView, nil
}

// List returns all views for the given form ordered by order ASC.
func (s *Service) List(ctx context.Context, formID uuid.UUID) ([]View, error) {
	traceCtx, span := s.tracer.Start(ctx, "List")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	exist, err := s.queries.FormExists(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check form exists")
		span.RecordError(err)
		return nil, err
	}
	if !exist {
		return nil, internal.ErrFormNotFound
	}

	views, err := s.queries.ListByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list views")
		span.RecordError(err)
		return nil, err
	}

	return views, nil
}

// Get returns a single view by formID and viewID.
func (s *Service) Get(ctx context.Context, formID, viewID uuid.UUID) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	v, err := s.queries.Get(traceCtx, GetParams{
		ID:     viewID,
		FormID: formID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "get view")
		span.RecordError(err)
		return View{}, err
	}

	return v, nil
}

// UpdateTitle renames the view.
func (s *Service) UpdateTitle(ctx context.Context, formID, viewID uuid.UUID, title string) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateTitle")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	v, err := s.queries.UpdateTitle(traceCtx, UpdateTitleParams{
		ID:     viewID,
		FormID: formID,
		Title:  title,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		if isUniqueViolation(err) {
			return View{}, internal.ErrViewNameDuplicate
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "update view title")
		span.RecordError(err)
		return View{}, err
	}

	return v, nil
}

// UpdateOrder moves the view to newOrder, shifting other views as needed.
func (s *Service) UpdateOrder(ctx context.Context, formID, viewID uuid.UUID, newOrder int32) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "UpdateOrder")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	maxOrder, err := s.queries.MaxOrder(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get max order for update")
		span.RecordError(err)
		return View{}, err
	}
	if newOrder > maxOrder || newOrder < 0 {
		return View{}, internal.ErrViewOrderOutOfRange
	}

	row, err := s.queries.UpdateOrder(traceCtx, UpdateOrderParams{
		FormID: formID,
		ID:     viewID,
		Order:  newOrder,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "update view order")
		span.RecordError(err)
		return View{}, err
	}

	return View(row), nil
}

// Lock sets locked=true on the view.
func (s *Service) Lock(ctx context.Context, formID, viewID uuid.UUID) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "Lock")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	v, err := s.queries.UpdateLocked(traceCtx, UpdateLockedParams{
		ID:     viewID,
		FormID: formID,
		Locked: true,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "lock view")
		span.RecordError(err)
		return View{}, err
	}

	return v, nil
}

// Unlock sets locked=false on the view.
func (s *Service) Unlock(ctx context.Context, formID, viewID uuid.UUID) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "Unlock")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	v, err := s.queries.UpdateLocked(traceCtx, UpdateLockedParams{
		ID:     viewID,
		FormID: formID,
		Locked: false,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "unlock view")
		span.RecordError(err)
		return View{}, err
	}

	return v, nil
}

// Duplicate creates a copy of the view with title, placed immediately after the original in order.
func (s *Service) Duplicate(ctx context.Context, formID, viewID uuid.UUID) (View, error) {
	traceCtx, span := s.tracer.Start(ctx, "Duplicate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	beginner, ok := s.db.(TxBeginner)
	if !ok {
		return View{}, fmt.Errorf("db does not support transactions")
	}

	tx, err := beginner.BeginTx(traceCtx, pgx.TxOptions{})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "begin tx")
		span.RecordError(err)
		return View{}, err
	}
	defer func() {
		if rbErr := tx.Rollback(traceCtx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.Error("rollback failed", zap.Error(rbErr))
		}
	}()

	qtx := s.queries.WithTx(tx)

	original, err := qtx.Get(traceCtx, GetParams{
		ID:     viewID,
		FormID: formID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return View{}, internal.ErrViewNotFound
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "get view for duplicate")
		span.RecordError(err)
		return View{}, err
	}

	titles, err := qtx.ListTitlesByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list view titles for duplicate")
		span.RecordError(err)
		return View{}, err
	}
	newTitle := generateDuplicateTitle(original.Title, titles)

	// Shift views after inserted position backward (order + 1)
	err = qtx.ShiftOrders(traceCtx, ShiftOrdersParams{
		FormID:  formID,
		Order:   original.Order,
		Order_2: 1,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "shift orders for duplicate")
		span.RecordError(err)
		return View{}, err
	}

	newView, err := qtx.Create(traceCtx, CreateParams{
		FormID: formID,
		Title:  newTitle,
		Order:  original.Order + 1,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create duplicate view")
		span.RecordError(err)
		return View{}, err
	}

	err = tx.Commit(traceCtx)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "commit tx")
		span.RecordError(err)
		return View{}, err
	}

	return newView, nil
}

// Delete deletes the view and move the views after deleted position order forward.
func (s *Service) Delete(ctx context.Context, formID, viewID uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	deleted, err := s.queries.DeleteIfUnlocked(traceCtx, DeleteIfUnlockedParams{
		ID:     viewID,
		FormID: formID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			exist, exErr := s.queries.Exists(traceCtx, ExistsParams{
				ID:     viewID,
				FormID: formID,
			})
			if exErr != nil {
				exErr = databaseutil.WrapDBErrorWithKeyValue(exErr, "view", "id", viewID.String(), logger, "check view exists after delete failed")
				span.RecordError(exErr)
				return exErr
			}
			if !exist {
				return internal.ErrViewNotFound
			}
			return internal.ErrViewLocked
		}
		err = databaseutil.WrapDBErrorWithKeyValue(err, "view", "id", viewID.String(), logger, "delete view")
		span.RecordError(err)
		return err
	}

	err = s.queries.ShiftOrders(traceCtx, ShiftOrdersParams{
		FormID:  formID,
		Order:   deleted.Order,
		Order_2: -1,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "shift orders for delete")
		span.RecordError(err)
		return err
	}

	return nil
}
