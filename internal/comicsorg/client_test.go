package comicsorg

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
)

type memoryCache struct {
	mu    sync.Mutex
	items map[string][]byte
}

func (m *memoryCache) Get(_ context.Context, key string, _ time.Duration) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	payload, ok := m.items[key]
	return payload, ok, nil
}

func (m *memoryCache) Put(_ context.Context, key string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = payload
	return nil
}

const asterixList = `{"count": 3, "results": [
  {"api_url": "BASE/api/issue/10/?format=json", "series_name": "Astérix (1961 series)",
   "descriptor": "5 - Le tour de Gaule", "publication_date": "1965", "variant_of": null},
  {"api_url": "BASE/api/issue/11/?format=json", "series_name": "Astérix (1961 series)",
   "descriptor": "5 - Variant", "publication_date": "1965", "variant_of": "BASE/api/issue/10/?format=json"},
  {"api_url": "BASE/api/issue/12/?format=json", "series_name": "Asterix (1971 series)",
   "descriptor": "5 - Die goldene Sichel", "publication_date": "1971", "variant_of": null}
]}`

const issue10 = `{"title": "Le tour de Gaule", "publication_date": "1965", "cover": "https://files.test/10.jpg"}`

// newAPI serves a fake comics.org API. Detail requests for issue 12 fail, so
// the candidate without details must still be returned.
// hits counts requests for the Asterix #5 search list only.
func newAPI(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := server.URL
		switch {
		case r.URL.Path == "/api/series/name/Asterix/issue/5/" && r.URL.RawQuery == "format=json":
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, strings.ReplaceAll(asterixList, "BASE", base))
		case r.URL.Path == "/api/series/name/Asterix/issue/5/year/1965/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"count": 0, "results": []}`)
		case r.URL.Path == "/api/issue/10/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, issue10)
		case r.URL.Path == "/api/issue/12/":
			http.Error(w, "boom", http.StatusInternalServerError)
		case strings.HasPrefix(r.URL.Path, "/api/series/name/Broken/"):
			http.Error(w, "boom", http.StatusServiceUnavailable)
		case strings.HasPrefix(r.URL.Path, "/api/series/name/Busy/"):
			http.Error(w, "slow down", http.StatusTooManyRequests)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newClient(baseURL string, cache Cache) *Client {
	return New(Options{BaseURL: baseURL, Cache: cache, Logger: slog.New(slog.DiscardHandler)})
}

func TestSearchReturnsCandidatesWithDetailsAndSkipsVariants(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", &memoryCache{items: map[string][]byte{}})

	got, err := client.Search(context.Background(), catalog.Query{Series: "Asterix", Number: "5"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Search() returned %d candidates, want 2 (variant skipped): %+v", len(got), got)
	}

	first := got[0]
	want := catalog.Candidate{
		Source:          catalog.SourceComicsOrg,
		SourceID:        "10",
		SourceURL:       "https://www.comics.org/issue/10/",
		SeriesTitle:     "Astérix (1961 series)",
		Title:           "Le tour de Gaule",
		PublicationDate: "1965",
		CoverURL:        "https://files.test/10.jpg",
	}
	if first != want {
		t.Errorf("first candidate = %+v\nwant %+v", first, want)
	}

	second := got[1]
	if second.SourceID != "12" || second.CoverURL != "" || second.Title != "5 - Die goldene Sichel" {
		t.Errorf("candidate without detail = %+v", second)
	}
}

func TestSearchUsesYearFilter(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", nil)

	got, err := client.Search(context.Background(), catalog.Query{Series: "Asterix", Number: "5", Year: "1965"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Search() with year returned %d candidates, want 0", len(got))
	}
}

func TestSearchCachesResponses(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", &memoryCache{items: map[string][]byte{}})
	query := catalog.Query{Series: "Asterix", Number: "5"}

	if _, err := client.Search(context.Background(), query); err != nil {
		t.Fatalf("first Search() error = %v", err)
	}
	firstHits := hits.Load()
	if _, err := client.Search(context.Background(), query); err != nil {
		t.Fatalf("second Search() error = %v", err)
	}
	if hits.Load() != firstHits {
		t.Errorf("second search made %d requests, want none (cached)", hits.Load()-firstHits)
	}
}

func TestSearchReportsUnavailableSource(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", nil)

	_, err := client.Search(context.Background(), catalog.Query{Series: "Broken", Number: "1"})
	var unavailableErr *catalog.UnavailableError
	if !errors.As(err, &unavailableErr) {
		t.Fatalf("Search() error = %v, want *catalog.UnavailableError", err)
	}
	if got := unavailableErr.Error(); got != "comics.org responded with HTTP 503" {
		t.Errorf("message = %q", got)
	}
}

func TestSearchExplainsRateLimiting(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", nil)

	_, err := client.Search(context.Background(), catalog.Query{Series: "Busy", Number: "1"})
	var unavailableErr *catalog.UnavailableError
	if !errors.As(err, &unavailableErr) || !strings.Contains(unavailableErr.Reason, "rate-limiting") {
		t.Fatalf("Search() error = %v, want a rate-limiting explanation", err)
	}
}

func TestSearchTreatsUnknownSeriesAsNoMatches(t *testing.T) {
	var hits atomic.Int32
	server := newAPI(t, &hits)
	client := newClient(server.URL+"/api", nil)

	got, err := client.Search(context.Background(), catalog.Query{Series: "Nothing Like This", Number: "1"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Search() = %+v, want no candidates", got)
	}
}

func TestSearchReportsUnreachableSource(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	baseURL := server.URL
	server.Close()

	client := newClient(baseURL+"/api", nil)
	_, err := client.Search(context.Background(), catalog.Query{Series: "Asterix", Number: "5"})
	var unavailableErr *catalog.UnavailableError
	if !errors.As(err, &unavailableErr) || unavailableErr.Reason != "could not be reached" {
		t.Fatalf("Search() error = %v, want unreachable source", err)
	}
}
