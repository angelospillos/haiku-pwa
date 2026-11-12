// Package collection stores the issues a user owns and their photos.
package collection

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ErrNotFound is returned when an issue or its photo does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when the issue is already in the collection.
var ErrDuplicate = errors.New("this issue is already in the collection")

// DuplicateError carries the issue that is already stored.
type DuplicateError struct {
	Existing Comic
}

func (e *DuplicateError) Error() string { return ErrDuplicate.Error() }

// Unwrap lets callers match the error with errors.Is(err, ErrDuplicate).
func (e *DuplicateError) Unwrap() error { return ErrDuplicate }

// Comic is an issue in the collection.
type Comic struct {
	ID              int64
	Series          string
	Number          string
	Source          string
	SourceID        string
	SourceURL       string
	SeriesTitle     string
	Title           string
	PublicationDate string
	CoverURL        string
	PhotoFile       string
	CreatedAt       time.Time
}

// NewComic describes an issue to add. Match is nil when the user saved it
// without choosing a catalogue entry.
type NewComic struct {
	Series    string
	Number    string
	PhotoData []byte
	PhotoExt  string
	Match     *catalog.Candidate
}

// SeriesKey normalises a series name for comparison: case, accents and
// repeated whitespace are ignored.
func SeriesKey(series string) string {
	folded, _, err := transform.String(accentFolder, series)
	if err != nil {
		folded = series
	}
	return strings.Join(strings.Fields(strings.ToLower(folded)), " ")
}

var accentFolder = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// NumberKey normalises an issue number for comparison: whitespace is removed,
// letters are upper-cased and leading zeros are dropped, so "05" equals "5".
func NumberKey(number string) string {
	key := strings.ToUpper(strings.Join(strings.Fields(number), ""))
	for len(key) > 1 && key[0] == '0' && key[1] >= '0' && key[1] <= '9' {
		key = key[1:]
	}
	return key
}

// Store persists the collection in SQLite and photos on disk.
type Store struct {
	db        *sql.DB
	photosDir string
}

// NewStore returns a Store that keeps photos in photosDir.
func NewStore(db *sql.DB, photosDir string) *Store {
	return &Store{db: db, photosDir: photosDir}
}

const selectColumns = `id, series, number,
	COALESCE(source, ''), COALESCE(source_id, ''), COALESCE(source_url, ''),
	COALESCE(series_title, ''), COALESCE(title, ''), COALESCE(publication_date, ''),
	COALESCE(cover_url, ''), photo_file, created_at`

// List returns every issue ordered by series, then numerically by issue number.
func (s *Store) List(ctx context.Context) ([]Comic, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+selectColumns+`
		FROM comics ORDER BY series_key, CAST(number_key AS INTEGER), number_key`)
	if err != nil {
		return nil, fmt.Errorf("list collection: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comics []Comic
	for rows.Next() {
		comic, err := scanComic(rows)
		if err != nil {
			return nil, fmt.Errorf("list collection: %w", err)
		}
		comics = append(comics, comic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list collection: %w", err)
	}
	return comics, nil
}

// SeriesNames returns the distinct series names in the collection.
func (s *Store) SeriesNames(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT series FROM comics ORDER BY series`)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list series: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// Find returns the issue matching series and number, or ErrNotFound.
func (s *Store) Find(ctx context.Context, series, number string) (Comic, error) {
	return findByKey(ctx, s.db, SeriesKey(series), NumberKey(number))
}

// Get returns the issue with the given id, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (Comic, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM comics WHERE id = ?`, id)
	comic, err := scanComic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Comic{}, ErrNotFound
	}
	return comic, err
}

// PhotoPath returns the absolute path of the issue's photo, or ErrNotFound.
func (s *Store) PhotoPath(ctx context.Context, id int64) (string, error) {
	comic, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.photosDir, comic.PhotoFile), nil
}

// Add stores a new issue and its photo. It returns *DuplicateError when the
// issue is already present; the existing issue is included in the error.
func (s *Store) Add(ctx context.Context, in NewComic) (Comic, error) {
	seriesKey, numberKey := SeriesKey(in.Series), NumberKey(in.Number)
	photoFile, err := newPhotoFileName(in.PhotoExt)
	if err != nil {
		return Comic{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Comic{}, fmt.Errorf("add to collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := findByKey(ctx, tx, seriesKey, numberKey)
	switch {
	case err == nil:
		return Comic{}, &DuplicateError{Existing: existing}
	case !errors.Is(err, ErrNotFound):
		return Comic{}, fmt.Errorf("add to collection: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO comics (series, series_key, number, number_key, source, source_id, source_url,
			series_title, title, publication_date, cover_url, photo_file, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Series, seriesKey, in.Number, numberKey,
		matchField(in.Match, func(c *catalog.Candidate) string { return c.Source }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.SourceID }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.SourceURL }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.SeriesTitle }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.Title }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.PublicationDate }),
		matchField(in.Match, func(c *catalog.Candidate) string { return c.CoverURL }),
		photoFile,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return Comic{}, fmt.Errorf("add to collection: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Comic{}, fmt.Errorf("add to collection: %w", err)
	}

	photoPath := filepath.Join(s.photosDir, photoFile)
	if err := writeFileAtomic(photoPath, in.PhotoData); err != nil {
		return Comic{}, fmt.Errorf("save photo: %w", err)
	}
	if err := tx.Commit(); err != nil {
		_ = os.Remove(photoPath)
		return Comic{}, fmt.Errorf("add to collection: %w", err)
	}
	return s.Get(ctx, id)
}

// Remove deletes the issue and its photo. It returns ErrNotFound when the
// issue does not exist.
func (s *Store) Remove(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove from collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var photoFile string
	err = tx.QueryRowContext(ctx, `SELECT photo_file FROM comics WHERE id = ?`, id).Scan(&photoFile)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove from collection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM comics WHERE id = ?`, id); err != nil {
		return fmt.Errorf("remove from collection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("remove from collection: %w", err)
	}
	if err := os.Remove(filepath.Join(s.photosDir, photoFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove photo: %w", err)
	}
	return nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func findByKey(ctx context.Context, q rowQuerier, seriesKey, numberKey string) (Comic, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+selectColumns+` FROM comics WHERE series_key = ? AND number_key = ?`,
		seriesKey, numberKey)
	comic, err := scanComic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Comic{}, ErrNotFound
	}
	return comic, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanComic(row scanner) (Comic, error) {
	var comic Comic
	var createdAt string
	err := row.Scan(
		&comic.ID, &comic.Series, &comic.Number,
		&comic.Source, &comic.SourceID, &comic.SourceURL,
		&comic.SeriesTitle, &comic.Title, &comic.PublicationDate,
		&comic.CoverURL, &comic.PhotoFile, &createdAt,
	)
	if err != nil {
		return Comic{}, err
	}
	comic.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Comic{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	return comic, nil
}

func matchField(match *catalog.Candidate, pick func(*catalog.Candidate) string) string {
	if match == nil {
		return ""
	}
	return pick(match)
}

func newPhotoFileName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate photo name: %w", err)
	}
	return hex.EncodeToString(buf) + "." + ext, nil
}

// writeFileAtomic writes to a temporary file and renames it into place, so a
// crash never leaves a half-written photo under its final name.
func writeFileAtomic(name string, data []byte) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
