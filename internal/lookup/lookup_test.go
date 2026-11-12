package lookup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
)

type fakeSearcher struct {
	candidates []catalog.Candidate
	err        error
}

func (f fakeSearcher) Search(context.Context, catalog.Query) ([]catalog.Candidate, error) {
	return f.candidates, f.err
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLookupMergesSourcesInOrder(t *testing.T) {
	comics := catalog.Candidate{Source: catalog.SourceComicsOrg, SourceID: "10"}
	inducks := catalog.Candidate{Source: catalog.SourceInducks, SourceID: "it/TL 3401"}
	service := NewService(quietLogger(),
		Source{Name: catalog.SourceComicsOrg, Searcher: fakeSearcher{candidates: []catalog.Candidate{comics}}},
		Source{Name: catalog.SourceInducks, Searcher: fakeSearcher{candidates: []catalog.Candidate{inducks}}},
	)

	result := service.Lookup(context.Background(), catalog.Query{Series: "Topolino", Number: "3401"})
	if len(result.Candidates) != 2 || result.Candidates[0] != comics || result.Candidates[1] != inducks {
		t.Errorf("Candidates = %+v, want comics.org then inducks", result.Candidates)
	}
	if s := result.Statuses[catalog.SourceComicsOrg]; !s.OK || s.Count != 1 {
		t.Errorf("comics.org status = %+v", s)
	}
}

func TestLookupKeepsWorkingSourcesWhenOneFails(t *testing.T) {
	inducks := catalog.Candidate{Source: catalog.SourceInducks, SourceID: "it/TL 3401"}
	failing := &catalog.UnavailableError{Source: catalog.SourceComicsOrg, Reason: "responded with HTTP 500"}
	service := NewService(quietLogger(),
		Source{Name: catalog.SourceComicsOrg, Searcher: fakeSearcher{err: failing}},
		Source{Name: catalog.SourceInducks, Searcher: fakeSearcher{candidates: []catalog.Candidate{inducks}}},
	)

	result := service.Lookup(context.Background(), catalog.Query{Series: "Topolino", Number: "3401"})
	if len(result.Candidates) != 1 || result.Candidates[0] != inducks {
		t.Errorf("Candidates = %+v, want only the inducks match", result.Candidates)
	}
	status := result.Statuses[catalog.SourceComicsOrg]
	if status.OK || status.Message != "comics.org responded with HTTP 500" {
		t.Errorf("comics.org status = %+v", status)
	}
}

func TestLookupHidesUnexpectedErrorDetails(t *testing.T) {
	service := NewService(quietLogger(),
		Source{Name: "broken", Searcher: fakeSearcher{err: errors.New("disk on fire at /secret/path")}},
	)

	result := service.Lookup(context.Background(), catalog.Query{Series: "X", Number: "1"})
	status := result.Statuses["broken"]
	if status.OK || status.Message != "lookup failed" {
		t.Errorf("status = %+v, want generic message", status)
	}
}

func TestLookupReportsEmptySourcesAsOK(t *testing.T) {
	service := NewService(quietLogger(),
		Source{Name: catalog.SourceInducks, Searcher: fakeSearcher{}},
	)
	result := service.Lookup(context.Background(), catalog.Query{Series: "X", Number: "1"})
	if s := result.Statuses[catalog.SourceInducks]; !s.OK || s.Count != 0 {
		t.Errorf("status = %+v, want ok with zero matches", s)
	}
}
