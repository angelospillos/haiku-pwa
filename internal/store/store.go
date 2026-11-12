// Package store opens the SQLite databases, applies schema migrations and
// provides the HTTP response cache shared by the metadata clients.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

//go:embed migrations/*.sql
var collectionMigrations embed.FS

// OpenCollection opens the collection database, creating it if needed, and
// applies any pending migrations. The database runs in WAL mode so the API can
// keep serving reads while a write is in progress.
func OpenCollection(ctx context.Context, file string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", dsn(file,
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(ON)",
		"_pragma=journal_mode(WAL)",
		"_txlock=immediate",
	))
	if err != nil {
		return nil, fmt.Errorf("open collection database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open collection database: %w", err)
	}
	if err := migrate(ctx, db, collectionMigrations, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate collection database: %w", err)
	}
	return db, nil
}

// OpenReadOnly opens an existing database without allowing writes.
func OpenReadOnly(ctx context.Context, file string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(file, "_pragma=busy_timeout(5000)", "_pragma=query_only(ON)"))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	return db, nil
}

// Create opens file for writing, creating it if needed. No migrations are
// applied, so the caller owns the schema. It is meant for databases that are
// rebuilt from scratch, which is why durability is traded for import speed.
func Create(ctx context.Context, file string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(file, "_pragma=busy_timeout(5000)", "_pragma=synchronous(OFF)"))
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", file, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create %s: %w", file, err)
	}
	return db, nil
}

func dsn(file string, params ...string) string {
	return "file:" + file + "?" + strings.Join(params, "&")
}

func migrate(ctx context.Context, db *sql.DB, fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	for _, name := range names {
		version, err := versionOf(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		script, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return err
		}
		if err := apply(ctx, db, version, string(script)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		current = version
	}
	return nil
}

func versionOf(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must start with a version number", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %q must start with a version number", name)
	}
	return version, nil
}

func apply(ctx context.Context, db *sql.DB, version int, script string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, script); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// Cache stores response bodies by key with the time they were fetched.
type Cache struct {
	db *sql.DB
}

// NewCache returns a cache backed by the collection database.
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Get returns the cached payload when it is younger than maxAge.
func (c *Cache) Get(ctx context.Context, key string, maxAge time.Duration) ([]byte, bool, error) {
	var payload []byte
	var fetchedAt int64
	err := c.db.QueryRowContext(ctx,
		`SELECT payload, fetched_at FROM http_cache WHERE cache_key = ?`, key,
	).Scan(&payload, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if time.Since(time.UnixMilli(fetchedAt)) > maxAge {
		return nil, false, nil
	}
	return payload, true, nil
}

// Put stores payload under key, replacing any previous entry.
func (c *Cache) Put(ctx context.Context, key string, payload []byte) error {
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO http_cache (cache_key, payload, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT (cache_key) DO UPDATE SET payload = excluded.payload, fetched_at = excluded.fetched_at`,
		key, payload, time.Now().UnixMilli())
	return err
}
