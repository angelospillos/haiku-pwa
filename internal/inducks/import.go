package inducks

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/store"
)

// Members lists the files from the Inducks dump (isv/ inside isv.tgz) that the
// index needs. The rest of the dump is ignored.
var Members = []string{
	"inducks_issue.isv",
	"inducks_publication.isv",
	"inducks_publicationname.isv",
	"inducks_countryname.isv",
}

const maxLineBytes = 1 << 20

var schema = []string{
	`CREATE TABLE country (countrycode TEXT PRIMARY KEY, countryname TEXT NOT NULL)`,
	`CREATE TABLE publication (publicationcode TEXT PRIMARY KEY, countrycode TEXT, title TEXT NOT NULL)`,
	`CREATE TABLE publication_name (publicationcode TEXT NOT NULL, publicationname TEXT NOT NULL)`,
	`CREATE TABLE issue (
		issuecode TEXT PRIMARY KEY,
		publicationcode TEXT NOT NULL,
		issuenumber TEXT NOT NULL,
		title TEXT,
		oldestdate TEXT
	)`,
	`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
}

// indexes are created after the bulk insert, which is much faster than
// maintaining them row by row.
var indexes = []string{
	`CREATE INDEX issue_number ON issue (issuenumber)`,
	`CREATE INDEX publication_name_code ON publication_name (publicationcode)`,
}

// Counts reports how many rows an import stored.
type Counts struct {
	Countries        int
	Publications     int
	PublicationNames int
	Issues           int
}

// Import builds the index at dbPath from the .isv files in dir. The index is
// written to a temporary file and renamed into place only after the whole
// import succeeds, so readers never see a partial index.
func Import(ctx context.Context, dir, dbPath string) (Counts, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return Counts{}, fmt.Errorf("create data directory: %w", err)
	}
	tmpPath := dbPath + ".tmp"
	_ = os.Remove(tmpPath)

	db, err := store.Create(ctx, tmpPath)
	if err != nil {
		return Counts{}, err
	}
	counts, err := build(ctx, db, dir)
	closeErr := db.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return Counts{}, err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return Counts{}, fmt.Errorf("close index: %w", closeErr)
	}
	if err := os.Rename(tmpPath, dbPath); err != nil {
		_ = os.Remove(tmpPath)
		return Counts{}, fmt.Errorf("install index: %w", err)
	}
	return counts, nil
}

func build(ctx context.Context, db *sql.DB, dir string) (Counts, error) {
	for _, stmt := range schema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return Counts{}, fmt.Errorf("create schema: %w", err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Counts{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var counts Counts
	importers := []func(context.Context, *sql.Tx, string, *Counts) error{
		importCountries,
		importPublications,
		importPublicationNames,
		importIssues,
	}
	for _, importer := range importers {
		if err := importer(ctx, tx, dir, &counts); err != nil {
			return Counts{}, err
		}
	}

	meta := [][2]string{
		{"synced_at", time.Now().UTC().Format(time.RFC3339)},
		{"issues", fmt.Sprint(counts.Issues)},
		{"publications", fmt.Sprint(counts.Publications)},
	}
	for _, entry := range meta {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)`, entry[0], entry[1]); err != nil {
			return Counts{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Counts{}, err
	}

	for _, stmt := range indexes {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return Counts{}, fmt.Errorf("create index: %w", err)
		}
	}
	return counts, nil
}

func importCountries(ctx context.Context, tx *sql.Tx, dir string, counts *Counts) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO country (countrycode, countryname) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	return forEachRow(ctx, dir, "inducks_countryname.isv", func(f []string) error {
		// Columns: countrycode, languagecode, countryname. English names only.
		if len(f) < 3 || f[1] != "en" || f[0] == "" || f[2] == "" {
			return nil
		}
		n, err := execRows(ctx, stmt, f[0], f[2])
		counts.Countries += n
		return err
	})
}

func importPublications(ctx context.Context, tx *sql.Tx, dir string, counts *Counts) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO publication (publicationcode, countrycode, title) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	return forEachRow(ctx, dir, "inducks_publication.isv", func(f []string) error {
		// Columns: publicationcode, countrycode, languagecode, title, ...
		if len(f) < 4 || f[0] == "" || f[3] == "" {
			return nil
		}
		n, err := execRows(ctx, stmt, f[0], f[1], f[3])
		counts.Publications += n
		return err
	})
}

func importPublicationNames(ctx context.Context, tx *sql.Tx, dir string, counts *Counts) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO publication_name (publicationcode, publicationname) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	return forEachRow(ctx, dir, "inducks_publicationname.isv", func(f []string) error {
		if len(f) < 2 || f[0] == "" || f[1] == "" {
			return nil
		}
		n, err := execRows(ctx, stmt, f[0], f[1])
		counts.PublicationNames += n
		return err
	})
}

func importIssues(ctx context.Context, tx *sql.Tx, dir string, counts *Counts) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO issue (issuecode, publicationcode, issuenumber, title, oldestdate) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	return forEachRow(ctx, dir, "inducks_issue.isv", func(f []string) error {
		// Columns: issuecode, issuerangecode, publicationcode, issuenumber, title,
		// size, pages, price, printrun, attached, oldestdate, ...
		if len(f) < 11 || f[0] == "" || f[2] == "" || f[3] == "" {
			return nil
		}
		oldest := f[10]
		if oldest == "" || strings.HasPrefix(oldest, "9999") {
			oldest = ""
		}
		n, err := execRows(ctx, stmt, f[0], f[2], f[3], f[4], oldest)
		counts.Issues += n
		return err
	})
}

// execRows runs an insert and reports how many rows it actually stored. Inserts
// that hit an existing key are ignored by the statement and count as zero.
func execRows(ctx context.Context, stmt *sql.Stmt, args ...any) (int, error) {
	result, err := stmt.ExecContext(ctx, args...)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	return int(n), err
}

// forEachRow calls fn with the ^-separated fields of every data row in dir/name.
// The first line of each .isv file is a header and is skipped. Rows with too few
// fields, such as continuation lines of multi-line comments, are ignored.
func forEachRow(ctx context.Context, dir, name string, fn func(fields []string) error) error {
	file, err := os.Open(filepath.Join(dir, name)) //nolint:gosec // G304: dir is chosen by the operator and name is a constant
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	header := true
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if header {
			header = false
			continue
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		if err := fn(strings.Split(line, "^")); err != nil {
			return fmt.Errorf("import %s: %w", name, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}
