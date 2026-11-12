package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"github.com/angelospillos/haiku-pwa/internal/collection"
	"github.com/angelospillos/haiku-pwa/internal/lookup"
	"github.com/angelospillos/haiku-pwa/internal/store"
)

type fakeSource struct {
	candidates []catalog.Candidate
	err        error
}

func (f fakeSource) Search(context.Context, catalog.Query) ([]catalog.Candidate, error) {
	return f.candidates, f.err
}

type testEnv struct {
	handler   http.Handler
	photosDir string
}

func newTestEnv(t *testing.T, inducksAvailable bool) testEnv {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()

	db, err := store.OpenCollection(ctx, filepath.Join(dir, "collection.sqlite"))
	if err != nil {
		t.Fatalf("OpenCollection() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(webDir, 0o750); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(webDir, "index.html"), "<!doctype html><title>app</title>")
	mustWrite(t, filepath.Join(webDir, "sw.js"), "self.addEventListener('install', () => {})")

	photosDir := filepath.Join(dir, "photos")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	lookups := lookup.NewService(logger,
		lookup.Source{Name: catalog.SourceComicsOrg, Searcher: fakeSource{candidates: []catalog.Candidate{{
			Source:    catalog.SourceComicsOrg,
			SourceID:  "10",
			SourceURL: "https://www.comics.org/issue/10/",
			Title:     "Le tour de Gaule",
			CoverURL:  "https://files.test/10.jpg",
		}}}},
		lookup.Source{Name: catalog.SourceInducks, Searcher: fakeSource{err: &catalog.UnavailableError{
			Source: catalog.SourceInducks, Reason: "has not been synced yet (run make sync)",
		}}},
	)

	return testEnv{
		handler: NewHandler(Deps{
			Logger:           logger,
			Collection:       collection.NewStore(db, photosDir),
			Lookup:           lookups,
			InducksAvailable: inducksAvailable,
			WebDir:           webDir,
		}),
		photosDir: photosDir,
	}
}

func mustWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e testEnv) do(t *testing.T, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func jpegDataURL(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

type errorBody struct {
	Error string `json:"error"`
}

type itemBody struct {
	Item struct {
		ID        int64  `json:"id"`
		Series    string `json:"series"`
		Number    string `json:"number"`
		Source    string `json:"source"`
		SourceURL string `json:"sourceUrl"`
		CoverURL  string `json:"coverUrl"`
		PhotoURL  string `json:"photoUrl"`
	} `json:"item"`
}

func TestHealthReportsInducksAvailability(t *testing.T) {
	env := newTestEnv(t, false)
	rec := env.do(t, http.MethodGet, "/api/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		OK      bool `json:"ok"`
		Inducks bool `json:"inducks"`
	}
	decode(t, rec, &body)
	if !body.OK || body.Inducks {
		t.Errorf("health = %+v", body)
	}
}

func TestLookupMergesSourcesAndValidatesInput(t *testing.T) {
	env := newTestEnv(t, false)

	rec := env.do(t, http.MethodGet, "/api/lookup?series=Asterix&number=5&year=1965", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	var body struct {
		Candidates []catalog.Candidate `json:"candidates"`
		Sources    map[string]struct {
			OK      bool   `json:"ok"`
			Count   int    `json:"count"`
			Message string `json:"message"`
		} `json:"sources"`
	}
	decode(t, rec, &body)
	if len(body.Candidates) != 1 || body.Candidates[0].SourceID != "10" {
		t.Errorf("candidates = %+v", body.Candidates)
	}
	if s := body.Sources["comics.org"]; !s.OK || s.Count != 1 {
		t.Errorf("comics.org status = %+v", s)
	}
	if s := body.Sources["inducks"]; s.OK || !strings.Contains(s.Message, "not been synced") {
		t.Errorf("inducks status = %+v", s)
	}

	tests := map[string]struct {
		target string
		want   string
	}{
		"missing series": {"/api/lookup?number=5", "Series is required"},
		"missing number": {"/api/lookup?series=Asterix", "Issue number is required"},
		"bad year":       {"/api/lookup?series=Asterix&number=5&year=65", "Year must have four digits"},
		"long number":    {"/api/lookup?series=Asterix&number=" + strings.Repeat("9", 21), "Series and issue number must be shorter"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := env.do(t, http.MethodGet, tt.target, nil)
			var body errorBody
			decode(t, rec, &body)
			if rec.Code != http.StatusBadRequest || body.Error != tt.want {
				t.Errorf("status %d error %q, want 400 %q", rec.Code, body.Error, tt.want)
			}
		})
	}
}

func TestCollectionLifecycle(t *testing.T) {
	env := newTestEnv(t, false)

	empty := env.do(t, http.MethodGet, "/api/collection", nil)
	if !strings.Contains(empty.Body.String(), `"items":[]`) || !strings.Contains(empty.Body.String(), `"series":[]`) {
		t.Errorf("empty collection body = %s", empty.Body)
	}

	created := env.do(t, http.MethodPost, "/api/collection", map[string]any{
		"series": "  Asterix ",
		"number": "5",
		"photo":  jpegDataURL(t),
		"match": map[string]string{
			"source":    catalog.SourceComicsOrg,
			"sourceId":  "10",
			"sourceUrl": "https://www.comics.org/issue/10/",
			"title":     "Le tour de Gaule",
			"coverUrl":  "https://files.test/10.jpg",
		},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("POST status = %d body %s", created.Code, created.Body)
	}
	var saved itemBody
	decode(t, created, &saved)
	if saved.Item.Series != "Asterix" || saved.Item.Source != catalog.SourceComicsOrg || saved.Item.CoverURL == "" {
		t.Errorf("saved item = %+v", saved.Item)
	}

	photo := env.do(t, http.MethodGet, saved.Item.PhotoURL, nil)
	if photo.Code != http.StatusOK || photo.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo status = %d type %q", photo.Code, photo.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(photo.Body.Bytes(), []byte{0xff, 0xd8, 0xff}) {
		t.Error("photo is not a JPEG")
	}

	duplicate := env.do(t, http.MethodPost, "/api/collection", map[string]any{
		"series": "asterix", "number": "05", "photo": jpegDataURL(t),
	})
	var conflict struct {
		Error string `json:"error"`
		Item  struct {
			ID int64 `json:"id"`
		} `json:"item"`
	}
	decode(t, duplicate, &conflict)
	if duplicate.Code != http.StatusConflict || conflict.Error == "" || conflict.Item.ID != saved.Item.ID {
		t.Errorf("duplicate status %d body %+v", duplicate.Code, conflict)
	}

	check := env.do(t, http.MethodGet, "/api/collection/check?series=ASTERIX&number=5", nil)
	if !strings.Contains(check.Body.String(), `"owned":true`) {
		t.Errorf("check body = %s", check.Body)
	}
	missing := env.do(t, http.MethodGet, "/api/collection/check?series=Asterix&number=6", nil)
	if !strings.Contains(missing.Body.String(), `"owned":false`) {
		t.Errorf("check for missing issue body = %s", missing.Body)
	}

	list := env.do(t, http.MethodGet, "/api/collection", nil)
	if !strings.Contains(list.Body.String(), `"series":["Asterix"]`) {
		t.Errorf("list body = %s", list.Body)
	}

	removed := env.do(t, http.MethodDelete, "/api/comics/"+strconv.FormatInt(saved.Item.ID, 10), nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", removed.Code)
	}
	if rec := env.do(t, http.MethodGet, saved.Item.PhotoURL, nil); rec.Code != http.StatusNotFound {
		t.Errorf("photo after delete status = %d", rec.Code)
	}
	if rec := env.do(t, http.MethodDelete, "/api/comics/"+strconv.FormatInt(saved.Item.ID, 10), nil); rec.Code != http.StatusNotFound {
		t.Errorf("second DELETE status = %d", rec.Code)
	}
	if rec := env.do(t, http.MethodDelete, "/api/comics/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE with bad id status = %d", rec.Code)
	}
}

func TestAddRejectsInvalidRequests(t *testing.T) {
	env := newTestEnv(t, false)

	tests := map[string]struct {
		body   any
		status int
		want   string
	}{
		"missing series": {map[string]any{"number": "5", "photo": "x"}, 400, "Series is required"},
		"missing number": {map[string]any{"series": "Asterix", "photo": "x"}, 400, "Issue number is required"},
		"no photo":       {map[string]any{"series": "Asterix", "number": "5"}, 400, photoRequiredMessage},
		"text instead of image": {map[string]any{
			"series": "Asterix", "number": "5", "photo": "data:text/plain;base64,aGk=",
		}, 400, photoRequiredMessage},
		"not json": {"{not json", 400, "Invalid JSON"},
		"photo too large": {map[string]any{
			"series": "Asterix", "number": "5",
			"photo": "data:image/jpeg;base64," + strings.Repeat("A", 14<<20),
		}, 413, photoTooLargeMessage},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := env.do(t, http.MethodPost, "/api/collection", tt.body)
			var body errorBody
			decode(t, rec, &body)
			if rec.Code != tt.status || body.Error != tt.want {
				t.Errorf("status %d error %q, want %d %q", rec.Code, body.Error, tt.status, tt.want)
			}
		})
	}
}

func TestUnsafeMatchDetailsAreDropped(t *testing.T) {
	env := newTestEnv(t, false)
	rec := env.do(t, http.MethodPost, "/api/collection", map[string]any{
		"series": "Odd", "number": "1", "photo": jpegDataURL(t),
		"match": map[string]string{
			"source":    catalog.SourceComicsOrg,
			"sourceUrl": "javascript:alert(1)",
			"coverUrl":  "ftp://files.test/x.jpg",
		},
	})
	var saved itemBody
	decode(t, rec, &saved)
	if rec.Code != http.StatusCreated || saved.Item.SourceURL != "" || saved.Item.CoverURL != "" {
		t.Errorf("status %d item %+v; unsafe URLs must be dropped", rec.Code, saved.Item)
	}

	rec = env.do(t, http.MethodPost, "/api/collection", map[string]any{
		"series": "Odd", "number": "2", "photo": jpegDataURL(t),
		"match": map[string]string{"source": "some-other-site", "sourceUrl": "https://example.test/"},
	})
	decode(t, rec, &saved)
	if saved.Item.Source != "" || saved.Item.SourceURL != "" {
		t.Errorf("unknown source must be dropped, got %+v", saved.Item)
	}
}

func TestWebClientIsServedWithFallback(t *testing.T) {
	env := newTestEnv(t, false)

	tests := map[string]struct {
		target string
		want   string
	}{
		"root":        {"/", "<title>app</title>"},
		"deep link":   {"/some/route", "<title>app</title>"},
		"static file": {"/sw.js", "addEventListener"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := env.do(t, http.MethodGet, tt.target, nil)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("status %d body %q", rec.Code, rec.Body)
			}
		})
	}

	if rec := env.do(t, http.MethodPost, "/", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST / status = %d, want 405", rec.Code)
	}
	if rec := env.do(t, http.MethodGet, "/api/unknown", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown API route status = %d", rec.Code)
	}
}

func TestResponsesCarrySecurityHeaders(t *testing.T) {
	env := newTestEnv(t, false)
	rec := env.do(t, http.MethodGet, "/api/health", nil)

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID is missing")
	}
}
