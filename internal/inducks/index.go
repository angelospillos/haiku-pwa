// Package inducks searches the Inducks index of Disney comics. Inducks blocks
// automated access to its website, so the index is built from the official
// daily data dump by Import instead.
package inducks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"github.com/angelospillos/haiku-pwa/internal/store"
)

const maxCandidates = 20

// ErrNotSynced is returned by Open when the index file does not exist yet.
var ErrNotSynced = errors.New("inducks index not found")

// Index is a read-only view of the SQLite index built by Import.
type Index struct {
	db       *sql.DB
	syncedAt string
}

// Open opens the index at file. It returns ErrNotSynced when the file is missing.
func Open(ctx context.Context, file string) (*Index, error) {
	if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotSynced
	}
	db, err := store.OpenReadOnly(ctx, file)
	if err != nil {
		return nil, err
	}
	index := &Index{db: db}
	_ = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'synced_at'`).Scan(&index.syncedAt)
	return index, nil
}

// SyncedAt reports when the index was built, as an RFC 3339 timestamp.
func (i *Index) SyncedAt() string {
	if i == nil {
		return ""
	}
	return i.syncedAt
}

// Close releases the database handle. It is safe to call on a nil Index.
func (i *Index) Close() error {
	if i == nil {
		return nil
	}
	return i.db.Close()
}

const searchSQL = `
SELECT i.issuecode,
       COALESCE(i.title, ''),
       COALESCE(i.oldestdate, ''),
       p.title,
       COALESCE(c.countryname, '')
FROM issue i
JOIN publication p ON p.publicationcode = i.publicationcode
LEFT JOIN country c ON c.countrycode = p.countrycode
WHERE i.issuenumber = ?
  AND (
    p.title LIKE ? ESCAPE '\'
    OR p.publicationcode IN (
      SELECT publicationcode FROM publication_name WHERE publicationname LIKE ? ESCAPE '\'
    )
  )
  AND (? = '' OR i.oldestdate IS NULL OR i.oldestdate = '' OR substr(i.oldestdate, 1, 4) = ?)
ORDER BY (p.title LIKE ? ESCAPE '\') DESC, p.title, i.oldestdate
LIMIT ?`

// Search returns the issues whose publication name contains q.Series and whose
// issue number equals q.Number. A nil Index reports that the data is not synced.
func (i *Index) Search(ctx context.Context, q catalog.Query) ([]catalog.Candidate, error) {
	if i == nil {
		return nil, &catalog.UnavailableError{Source: catalog.SourceInducks, Reason: "has not been synced yet (run make sync)"}
	}

	term := escapeLike(q.Series)
	contains := "%" + term + "%"
	rows, err := i.db.QueryContext(ctx, searchSQL,
		q.Number, contains, contains, q.Year, q.Year, term+"%", maxCandidates)
	if err != nil {
		return nil, fmt.Errorf("search inducks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candidates []catalog.Candidate
	for rows.Next() {
		var code, issueTitle, oldestDate, publicationTitle, country string
		if err := rows.Scan(&code, &issueTitle, &oldestDate, &publicationTitle, &country); err != nil {
			return nil, fmt.Errorf("search inducks: %w", err)
		}
		seriesTitle := publicationTitle
		if country != "" {
			seriesTitle += " · " + country
		}
		candidates = append(candidates, catalog.Candidate{
			Source:          catalog.SourceInducks,
			SourceID:        code,
			SourceURL:       "https://inducks.org/issue.php?" + url.Values{"c": {code}}.Encode(),
			SeriesTitle:     catalog.CleanText(seriesTitle, 200),
			Title:           catalog.CleanText(issueTitle, 200),
			PublicationDate: oldestDate,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search inducks: %w", err)
	}
	return candidates, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(value string) string {
	return likeEscaper.Replace(value)
}
