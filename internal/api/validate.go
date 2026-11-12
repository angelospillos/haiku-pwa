package api

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
)

const (
	maxSeriesRunes = 100
	maxNumberRunes = 20
	maxYearRunes   = 10

	photoRequiredMessage = "A photo of the comic is required (JPEG, PNG or WebP, up to 10 MB)"
	photoTooLargeMessage = "Photo is too large (10 MB limit)"
)

var yearPattern = regexp.MustCompile(`^\d{4}$`)

// bounded collapses whitespace in value and reports whether the result fits in
// max characters. Over-long input is rejected rather than silently truncated.
func bounded(value string, limit int) (string, bool) {
	cleaned := strings.Join(strings.Fields(value), " ")
	return cleaned, utf8.RuneCountInString(cleaned) <= limit
}

// parseIssueQuery validates the series, number and optional year from a query
// string. It returns a user-facing problem description when the input is not
// acceptable.
func parseIssueQuery(values url.Values, allowYear bool) (catalog.Query, string) {
	series, seriesOK := bounded(values.Get("series"), maxSeriesRunes)
	number, numberOK := bounded(values.Get("number"), maxNumberRunes)
	var year string
	if allowYear {
		var yearOK bool
		year, yearOK = bounded(values.Get("year"), maxYearRunes)
		if !yearOK || (year != "" && !yearPattern.MatchString(year)) {
			return catalog.Query{}, "Year must have four digits"
		}
	}

	switch {
	case series == "":
		return catalog.Query{}, "Series is required"
	case number == "":
		return catalog.Query{}, "Issue number is required"
	case !seriesOK || !numberOK:
		return catalog.Query{}, "Series and issue number must be shorter"
	}
	return catalog.Query{Series: series, Number: number, Year: year}, ""
}

// sanitizeMatch keeps only the catalogue details the server can vouch for: a
// known source, and URLs that are plain http or https links. Anything else is
// dropped, so a crafted request cannot store a javascript: link.
func sanitizeMatch(in *matchRequest) *catalog.Candidate {
	if in == nil || (in.Source != catalog.SourceComicsOrg && in.Source != catalog.SourceInducks) {
		return nil
	}
	return &catalog.Candidate{
		Source:          in.Source,
		SourceID:        catalog.CleanText(in.SourceID, 80),
		SourceURL:       httpURL(in.SourceURL, 300),
		SeriesTitle:     catalog.CleanText(in.SeriesTitle, 200),
		Title:           catalog.CleanText(in.Title, 200),
		PublicationDate: catalog.CleanText(in.PublicationDate, 40),
		CoverURL:        httpURL(in.CoverURL, 500),
	}
}

func httpURL(value string, limit int) string {
	text, ok := bounded(value, limit)
	if !ok {
		return ""
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return text
}
