package inducks

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
)

const fixtureDir = "../../tests/fixtures/inducks/isv"

func importFixture(t *testing.T) *Index {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "inducks.sqlite")

	counts, err := Import(ctx, fixtureDir, dbPath)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if counts.Issues != 4 || counts.Publications != 2 || counts.Countries != 2 {
		t.Errorf("Import() counts = %+v", counts)
	}

	index, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return index
}

func TestImportAndSearch(t *testing.T) {
	index := importFixture(t)
	ctx := context.Background()

	if index.SyncedAt() == "" {
		t.Error("SyncedAt() is empty after import")
	}

	got, err := index.Search(ctx, catalog.Query{Series: "Topolino", Number: "3401"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	want := catalog.Candidate{
		Source:          catalog.SourceInducks,
		SourceID:        "it/TL 3401",
		SourceURL:       "https://inducks.org/issue.php?c=it%2FTL+3401",
		SeriesTitle:     "Topolino (libretto) · Italy",
		Title:           "",
		PublicationDate: "2021-01-27",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Search() = %+v, want [%+v]", got, want)
	}
}

func TestSearchMatchesIssueNumbersWithDashes(t *testing.T) {
	index := importFixture(t)

	got, err := index.Search(context.Background(), catalog.Query{Series: "Micky Maus", Number: "2001-03"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].SourceID != "de/MM2001-03" || got[0].SeriesTitle != "Micky Maus · Germany" {
		t.Fatalf("Search() = %+v", got)
	}
}

func TestSearchReturnsNothingForUnknownIssues(t *testing.T) {
	index := importFixture(t)
	ctx := context.Background()

	for _, q := range []catalog.Query{
		{Series: "Topolino", Number: "1"},
		{Series: "Asterix", Number: "5"},
	} {
		got, err := index.Search(ctx, q)
		if err != nil {
			t.Fatalf("Search(%+v) error = %v", q, err)
		}
		if len(got) != 0 {
			t.Errorf("Search(%+v) = %+v, want none", q, got)
		}
	}
}

func TestSearchFiltersByYear(t *testing.T) {
	index := importFixture(t)
	ctx := context.Background()

	got, err := index.Search(ctx, catalog.Query{Series: "Topolino", Number: "3401", Year: "2021"})
	if err != nil || len(got) != 1 {
		t.Fatalf("Search(year 2021) = %+v, %v; want one match", got, err)
	}
	got, err = index.Search(ctx, catalog.Query{Series: "Topolino", Number: "3401", Year: "1965"})
	if err != nil || len(got) != 0 {
		t.Fatalf("Search(year 1965) = %+v, %v; want none", got, err)
	}
}

func TestSearchTreatsLikeWildcardsAsText(t *testing.T) {
	index := importFixture(t)

	got, err := index.Search(context.Background(), catalog.Query{Series: "Top%", Number: "3401"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Search(\"Top%%\") = %+v, want no matches", got)
	}
}

func TestNilIndexReportsNotSynced(t *testing.T) {
	var index *Index
	_, err := index.Search(context.Background(), catalog.Query{Series: "Topolino", Number: "3401"})
	var unavailable *catalog.UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Search() on nil index error = %v, want UnavailableError", err)
	}
	if err := index.Close(); err != nil {
		t.Errorf("Close() on nil index error = %v", err)
	}
}

func TestOpenMissingIndexReturnsErrNotSynced(t *testing.T) {
	_, err := Open(context.Background(), filepath.Join(t.TempDir(), "missing.sqlite"))
	if !errors.Is(err, ErrNotSynced) {
		t.Fatalf("Open() error = %v, want ErrNotSynced", err)
	}
}
