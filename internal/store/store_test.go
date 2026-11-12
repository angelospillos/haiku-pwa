package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCollectionCreatesMissingDirectories(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "nested", "data", "collection.sqlite")

	for range 2 {
		db, err := OpenCollection(ctx, file)
		if err != nil {
			t.Fatalf("OpenCollection() error = %v", err)
		}
		var version int
		if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != 1 {
			t.Errorf("user_version = %d, want 1", version)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// A database written by the earlier Node version has the same tables but
// user_version 0. Opening it must keep its rows.
func TestOpenCollectionAdoptsExistingTables(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "collection.sqlite")

	legacy, err := sql.Open("sqlite", dsn(file))
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE comics (id INTEGER PRIMARY KEY AUTOINCREMENT, series TEXT NOT NULL, series_key TEXT NOT NULL,
			number TEXT NOT NULL, number_key TEXT NOT NULL, source TEXT, source_id TEXT, source_url TEXT,
			series_title TEXT, title TEXT, publication_date TEXT, cover_url TEXT, photo_file TEXT NOT NULL,
			created_at TEXT NOT NULL, UNIQUE (series_key, number_key))`,
		`CREATE TABLE http_cache (cache_key TEXT PRIMARY KEY, payload TEXT NOT NULL, fetched_at INTEGER NOT NULL)`,
		`INSERT INTO comics (series, series_key, number, number_key, photo_file, created_at)
			VALUES ('Asterix', 'asterix', '5', '5', 'legacy.jpg', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := legacy.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenCollection(ctx, file)
	if err != nil {
		t.Fatalf("OpenCollection() on legacy database error = %v", err)
	}
	defer func() { _ = db.Close() }()

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM comics`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("legacy rows = %d, want 1", count)
	}
}

func TestCacheExpiresEntries(t *testing.T) {
	ctx := context.Background()
	db, err := OpenCollection(ctx, filepath.Join(t.TempDir(), "collection.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)
	if err := cache.Put(ctx, "k", []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}

	payload, ok, err := cache.Get(ctx, "k", time.Hour)
	if err != nil || !ok || string(payload) != `{"a":1}` {
		t.Fatalf("Get() fresh = %q, %v, %v", payload, ok, err)
	}
	if _, ok, _ := cache.Get(ctx, "k", -time.Second); ok {
		t.Error("Get() returned an entry older than maxAge")
	}
	if _, ok, _ := cache.Get(ctx, "missing", time.Hour); ok {
		t.Error("Get() found a key that was never stored")
	}
}
