package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"api/internal/app/store"
	"api/internal/testutil"
)

func countTables(t *testing.T, opened *store.Store) int {
	t.Helper()
	var total int
	err := opened.Reads().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'",
	).Scan(&total)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("count tables: %v", err)
	}
	return total
}

func TestOpenStampsTheSchemaVersion(t *testing.T) {
	opened := testutil.NewStore(t)
	version, err := opened.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if version != store.Version() {
		t.Fatalf("version = %d, want %d", version, store.Version())
	}
}

func TestTheSchemaCreatesEveryTableItDeclares(t *testing.T) {
	opened := testutil.NewStore(t)
	if total := countTables(t, opened); total == 0 {
		t.Fatal("the schema created no table")
	}
}

func TestAVersionMismatchRecreatesTheSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	first := testutil.OpenStore(t, path)

	if _, err := first.Reads().ExecContext(
		context.Background(),
		"INSERT INTO settings (id, document, updated_at) VALUES (1, '{}', '2026-01-01T00:00:00Z')",
	); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if _, err := first.Reads().ExecContext(
		context.Background(),
		"PRAGMA user_version = 999",
	); err != nil {
		t.Fatalf("stamp a foreign version: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second := testutil.OpenStore(t, path)
	var rows int
	if err := second.Reads().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM settings",
	).Scan(&rows); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if rows != 0 {
		t.Fatalf("recreate kept %d settings rows", rows)
	}
	version, err := second.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if version != store.Version() {
		t.Fatalf("version = %d after recreate", version)
	}
}

func TestATransactionRollsBackOnFailure(t *testing.T) {
	opened := testutil.NewStore(t)
	wanted := errors.New("stop here")

	err := opened.WithTx(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			context.Background(),
			"INSERT INTO settings (id, document, updated_at) VALUES (1, '{}', '2026-01-01T00:00:00Z')",
		); err != nil {
			return err
		}
		return wanted
	})
	if !errors.Is(err, wanted) {
		t.Fatalf("err = %v, want %v", err, wanted)
	}

	var rows int
	if err := opened.Reads().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM settings",
	).Scan(&rows); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if rows != 0 {
		t.Fatalf("a failed transaction committed %d rows", rows)
	}
}

func TestATransactionCommitsOnSuccess(t *testing.T) {
	opened := testutil.NewStore(t)

	if err := opened.WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(
			context.Background(),
			"INSERT INTO settings (id, document, updated_at) VALUES (1, '{}', '2026-01-01T00:00:00Z')",
		)
		return err
	}); err != nil {
		t.Fatalf("transaction: %v", err)
	}

	var rows int
	if err := opened.Reads().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM settings",
	).Scan(&rows); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if rows != 1 {
		t.Fatalf("committed %d rows, want 1", rows)
	}
}
