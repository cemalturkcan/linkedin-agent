package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

const (
	driverName = "sqlite"
	dataMode   = os.FileMode(0o700)
)

var connectionPragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"foreign_keys(1)",
}

type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type TransactionRunner interface {
	WithTx(ctx context.Context, run func(tx *sql.Tx) error) error
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), dataMode); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open(driverName, dataSourceName(path))
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.apply(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return store, nil
}

func dataSourceName(path string) string {
	query := url.Values{}
	for _, pragma := range connectionPragmas {
		query.Add("_pragma", pragma)
	}
	return "file:" + path + "?" + query.Encode()
}

func (s *Store) Reads() DBTX {
	return s.db
}

func (s *Store) WithTx(ctx context.Context, run func(tx *sql.Tx) error) error {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	if err := run(transaction); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) apply(ctx context.Context) error {
	var found int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if found != schemaVersion {
		return s.recreate(ctx)
	}
	return nil
}

func (s *Store) recreate(ctx context.Context) error {
	return s.WithTx(ctx, func(transaction *sql.Tx) error {
		existing, err := tableNames(ctx, transaction)
		if err != nil {
			return err
		}
		for _, table := range existing {
			if _, err := transaction.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
				return fmt.Errorf("drop %s: %w", table, err)
			}
		}
		if _, err := transaction.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		if _, err := transaction.ExecContext(
			ctx,
			"PRAGMA user_version = "+strconv.Itoa(schemaVersion),
		); err != nil {
			return fmt.Errorf("stamp schema version: %w", err)
		}
		return nil
	})
}

func tableNames(ctx context.Context, transaction *sql.Tx) ([]string, error) {
	rows, err := transaction.QueryContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'",
	)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	names := make([]string, 0, 16)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	return names, nil
}

func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	return nil
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
