// Package catalog holds the types shared by the metadata sources: the query
// a user makes and the candidate issues a source returns for it.
package catalog

import "strings"

// Source names. They are stored with saved issues and shown to users.
const (
	SourceComicsOrg = "comics.org"
	SourceInducks   = "inducks"
)

// Query identifies an issue by series name and issue number, optionally
// narrowed by publication year.
type Query struct {
	Series string
	Number string
	Year   string
}

// UnavailableError reports that a source could not answer a query. Its message
// is written for end users, for example "comics.org responded with HTTP 503".
type UnavailableError struct {
	Source string
	Reason string
}

func (e *UnavailableError) Error() string {
	return e.Source + " " + e.Reason
}

// CleanText collapses runs of whitespace, trims the ends and truncates the
// result to maxRunes characters. It is applied to every string that comes from
// an external source or from a user before it is stored or displayed.
func CleanText(value string, maxRunes int) string {
	text := strings.Join(strings.Fields(value), " ")
	if runes := []rune(text); len(runes) > maxRunes {
		text = string(runes[:maxRunes])
	}
	return text
}

// Candidate is one possible match for a Query, as reported by a source.
type Candidate struct {
	Source          string `json:"source"`
	SourceID        string `json:"sourceId"`
	SourceURL       string `json:"sourceUrl"`
	SeriesTitle     string `json:"seriesTitle"`
	Title           string `json:"title"`
	PublicationDate string `json:"publicationDate"`
	CoverURL        string `json:"coverUrl"`
}
