package internal

import (
	"context"
	"errors"
	"testing"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

// stubDB implements DBTX but not TxBeginner, used to test unsupported-db handling.
type stubDB struct{}

func (stubDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (stubDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (stubDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

// trackTx is a pgx.Tx test double that records lifecycle calls and can inject errors.
type trackTx struct {
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (t *trackTx) Begin(context.Context) (pgx.Tx, error) {
	return t, nil
}

// Commit marks the tx as committed and returns commitErr when set.
func (t *trackTx) Commit(context.Context) error {
	t.committed = true
	return t.commitErr
}

// Rollback marks the tx as rolled back and returns rollbackErr when set.
func (t *trackTx) Rollback(context.Context) error {
	t.rolledBack = true
	return t.rollbackErr
}

func (t *trackTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (t *trackTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (t *trackTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (t *trackTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (t *trackTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (t *trackTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (t *trackTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (t *trackTx) Conn() *pgx.Conn { return nil }

// mockBeginner implements TxBeginner and returns a configured trackTx from BeginTx.
type mockBeginner struct {
	tx        *trackTx
	beginErr  error
	beginCall int
}

func (m *mockBeginner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockBeginner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBeginner) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

// BeginTx increments beginCall and returns tx or beginErr.
func (m *mockBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	m.beginCall++
	if m.beginErr != nil {
		return nil, m.beginErr
	}

	return m.tx, nil
}

// withTransactionFixture holds inputs and observable state for one subtest.
type withTransactionFixture struct {
	db       DBTX               // argument to WithTransaction
	fn       func(pgx.Tx) error // callback under test
	tx       *trackTx           // set when commit/rollback should be observed
	beginner *mockBeginner      // set when BeginTx behavior should be observed
}

func TestWithTransaction(t *testing.T) {
	t.Parallel()

	callbackErr := errors.New("callback failed")
	beginErr := errors.New("begin failed")
	commitErr := errors.New("commit failed")

	// --- Test cases ---
	testCases := []struct {
		name     string
		setup    func(t *testing.T) withTransactionFixture
		validate func(t *testing.T, err error, f withTransactionFixture)
	}{
		{
			name: "reuses existing tx",
			setup: func(t *testing.T) withTransactionFixture {
				tx := &trackTx{}
				return withTransactionFixture{
					db: tx,
					fn: func(got pgx.Tx) error {
						if got != tx {
							t.Fatal("expected same tx instance")
						}
						return nil
					},
					tx: tx,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.tx.committed || f.tx.rolledBack {
					t.Fatal("reused tx must not be committed or rolled back by helper")
				}
			},
		},
		{
			name: "commits new tx",
			setup: func(t *testing.T) withTransactionFixture {
				tx := &trackTx{rollbackErr: pgx.ErrTxClosed}
				beginner := &mockBeginner{tx: tx}
				return withTransactionFixture{
					db:       beginner,
					fn:       func(pgx.Tx) error { return nil },
					tx:       tx,
					beginner: beginner,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.beginner.beginCall != 1 {
					t.Fatalf("BeginTx calls = %d, want 1", f.beginner.beginCall)
				}
				if !f.tx.committed {
					t.Fatal("Commit not called")
				}
			},
		},
		{
			name: "returns callback error",
			setup: func(t *testing.T) withTransactionFixture {
				tx := &trackTx{}
				beginner := &mockBeginner{tx: tx}
				return withTransactionFixture{
					db:       beginner,
					fn:       func(pgx.Tx) error { return callbackErr },
					tx:       tx,
					beginner: beginner,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if !errors.Is(err, callbackErr) {
					t.Fatalf("got %v, want %v", err, callbackErr)
				}
				if f.tx.committed {
					t.Fatal("Commit should not run when callback fails")
				}
				if !f.tx.rolledBack {
					t.Fatal("Rollback not called")
				}
			},
		},
		{
			name: "returns callback error when rollback also fails",
			setup: func(t *testing.T) withTransactionFixture {
				rollbackErr := errors.New("rollback exploded")
				tx := &trackTx{rollbackErr: rollbackErr}
				beginner := &mockBeginner{tx: tx}
				return withTransactionFixture{
					db:       beginner,
					fn:       func(pgx.Tx) error { return callbackErr },
					tx:       tx,
					beginner: beginner,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if !errors.Is(err, callbackErr) {
					t.Fatalf("got %v, want callback error %v", err, callbackErr)
				}
				if f.tx.committed {
					t.Fatal("Commit should not run when callback fails")
				}
				if !f.tx.rolledBack {
					t.Fatal("Rollback not called")
				}
			},
		},
		{
			name: "reuses existing tx callback error",
			setup: func(t *testing.T) withTransactionFixture {
				tx := &trackTx{}
				return withTransactionFixture{
					db: tx,
					fn: func(pgx.Tx) error { return callbackErr },
					tx: tx,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if !errors.Is(err, callbackErr) {
					t.Fatalf("got %v, want %v", err, callbackErr)
				}
				if f.tx.committed || f.tx.rolledBack {
					t.Fatal("reused tx must not be committed or rolled back by helper")
				}
			},
		},
		{
			name: "unsupported db",
			setup: func(t *testing.T) withTransactionFixture {
				return withTransactionFixture{
					db: stubDB{},
					fn: func(pgx.Tx) error { return nil },
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				if !errors.Is(err, ErrDBTransactionNotSupported) {
					t.Fatalf("got %v, want %v", err, ErrDBTransactionNotSupported)
				}
			},
		},
		{
			name: "wraps begin failure",
			setup: func(t *testing.T) withTransactionFixture {
				beginner := &mockBeginner{beginErr: beginErr}
				return withTransactionFixture{
					db:       beginner,
					fn:       func(pgx.Tx) error { return nil },
					beginner: beginner,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				var wrapped databaseutil.InternalServerError
				if !errors.As(err, &wrapped) {
					t.Fatalf("expected wrapped db error, got %v", err)
				}
			},
		},
		{
			name: "wraps commit failure",
			setup: func(t *testing.T) withTransactionFixture {
				tx := &trackTx{commitErr: commitErr, rollbackErr: pgx.ErrTxClosed}
				beginner := &mockBeginner{tx: tx}
				return withTransactionFixture{
					db:       beginner,
					fn:       func(pgx.Tx) error { return nil },
					tx:       tx,
					beginner: beginner,
				}
			},
			validate: func(t *testing.T, err error, f withTransactionFixture) {
				var wrapped databaseutil.InternalServerError
				if !errors.As(err, &wrapped) {
					t.Fatalf("expected wrapped db error, got %v", err)
				}
				if !f.tx.committed {
					t.Fatal("Commit should have been attempted")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := tc.setup(t)
			err := WithTransaction(context.Background(), fixture.db, zap.NewNop(), fixture.fn)
			tc.validate(t, err, fixture)
		})
	}
}
