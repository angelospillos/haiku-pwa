// Package lookup asks every metadata source for an issue at the same time and
// reports each source's outcome, so one failing source never hides the others.
package lookup

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
)

// Searcher is implemented by every metadata source.
type Searcher interface {
	Search(ctx context.Context, q catalog.Query) ([]catalog.Candidate, error)
}

// Source pairs a Searcher with the name it is reported under.
type Source struct {
	Name     string
	Searcher Searcher
}

// Status is the outcome of one source for one lookup.
type Status struct {
	OK      bool
	Count   int
	Message string
}

// Result is the combined answer to a lookup.
type Result struct {
	Query      catalog.Query
	Candidates []catalog.Candidate
	Statuses   map[string]Status
}

// Service runs lookups against a fixed list of sources.
type Service struct {
	sources []Source
	logger  *slog.Logger
}

// NewService returns a Service that queries sources in the order given. Candidates
// are returned in the same order.
func NewService(logger *slog.Logger, sources ...Source) *Service {
	return &Service{sources: sources, logger: logger}
}

type outcome struct {
	candidates []catalog.Candidate
	err        error
}

// Lookup queries all sources concurrently and waits for every one of them.
func (s *Service) Lookup(ctx context.Context, q catalog.Query) Result {
	outcomes := make([]outcome, len(s.sources))
	var wg sync.WaitGroup
	for i, source := range s.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidates, err := source.Searcher.Search(ctx, q)
			outcomes[i] = outcome{candidates: candidates, err: err}
		}()
	}
	wg.Wait()

	result := Result{Query: q, Statuses: make(map[string]Status, len(s.sources))}
	for i, source := range s.sources {
		if err := outcomes[i].err; err != nil {
			result.Statuses[source.Name] = Status{Message: s.message(source.Name, err)}
			continue
		}
		candidates := outcomes[i].candidates
		result.Candidates = append(result.Candidates, candidates...)
		result.Statuses[source.Name] = Status{OK: true, Count: len(candidates)}
	}
	return result
}

// message returns text that is safe to show to users. Known unavailability is
// described; anything else is logged and reported generically.
func (s *Service) message(source string, err error) string {
	var unavailable *catalog.UnavailableError
	if errors.As(err, &unavailable) {
		return unavailable.Error()
	}
	s.logger.Error("lookup source failed", "source", source, "error", err)
	return "lookup failed"
}
