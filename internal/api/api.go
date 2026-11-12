// Package api serves the JSON API and the built web client.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"github.com/angelospillos/haiku-pwa/internal/collection"
	"github.com/angelospillos/haiku-pwa/internal/lookup"
	"github.com/angelospillos/haiku-pwa/internal/photo"
)

const maxBodyBytes = 15 << 20

// Deps are the collaborators the handler needs.
type Deps struct {
	Logger           *slog.Logger
	Collection       *collection.Store
	Lookup           *lookup.Service
	InducksAvailable bool
	WebDir           string
}

type handlers struct {
	deps Deps
}

// NewHandler returns the complete HTTP handler: API routes, the web client and
// the middleware that wraps them.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	h := &handlers{deps: deps}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/collection", h.listCollection)
	mux.HandleFunc("GET /api/collection/check", h.checkCollection)
	mux.HandleFunc("POST /api/collection", h.addComic)
	mux.HandleFunc("GET /api/comics/{id}/photo", h.comicPhoto)
	mux.HandleFunc("DELETE /api/comics/{id}", h.removeComic)
	mux.HandleFunc("GET /api/lookup", h.lookupIssue)
	mux.HandleFunc("/api/", notFound)
	mux.Handle("/", newWebHandler(deps.WebDir))

	return withMiddleware(deps.Logger, mux)
}

type healthResponse struct {
	OK      bool `json:"ok"`
	Inducks bool `json:"inducks"`
}

type comicDTO struct {
	ID              int64  `json:"id"`
	Series          string `json:"series"`
	Number          string `json:"number"`
	Source          string `json:"source"`
	SourceURL       string `json:"sourceUrl"`
	SeriesTitle     string `json:"seriesTitle"`
	Title           string `json:"title"`
	PublicationDate string `json:"publicationDate"`
	CoverURL        string `json:"coverUrl"`
	PhotoURL        string `json:"photoUrl"`
	CreatedAt       string `json:"createdAt"`
}

func toComicDTO(c collection.Comic) comicDTO {
	return comicDTO{
		ID:              c.ID,
		Series:          c.Series,
		Number:          c.Number,
		Source:          c.Source,
		SourceURL:       c.SourceURL,
		SeriesTitle:     c.SeriesTitle,
		Title:           c.Title,
		PublicationDate: c.PublicationDate,
		CoverURL:        c.CoverURL,
		PhotoURL:        "/api/comics/" + strconv.FormatInt(c.ID, 10) + "/photo",
		CreatedAt:       c.CreatedAt.Format(time.RFC3339Nano),
	}
}

type collectionResponse struct {
	Items  []comicDTO `json:"items"`
	Series []string   `json:"series"`
}

type checkResponse struct {
	Owned bool      `json:"owned"`
	Item  *comicDTO `json:"item"`
}

type addRequest struct {
	Series string        `json:"series"`
	Number string        `json:"number"`
	Photo  string        `json:"photo"`
	Match  *matchRequest `json:"match"`
}

type matchRequest struct {
	Source          string `json:"source"`
	SourceID        string `json:"sourceId"`
	SourceURL       string `json:"sourceUrl"`
	SeriesTitle     string `json:"seriesTitle"`
	Title           string `json:"title"`
	PublicationDate string `json:"publicationDate"`
	CoverURL        string `json:"coverUrl"`
}

type addResponse struct {
	Item comicDTO `json:"item"`
}

type conflictResponse struct {
	Error string   `json:"error"`
	Item  comicDTO `json:"item"`
}

type lookupResponse struct {
	Series     string                  `json:"series"`
	Number     string                  `json:"number"`
	Year       string                  `json:"year"`
	Candidates []catalog.Candidate     `json:"candidates"`
	Sources    map[string]sourceStatus `json:"sources"`
}

type sourceStatus struct {
	OK      bool   `json:"ok"`
	Count   int    `json:"count"`
	Message string `json:"message,omitempty"`
}

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{OK: true, Inducks: h.deps.InducksAvailable})
}

func (h *handlers) listCollection(w http.ResponseWriter, r *http.Request) {
	comics, err := h.deps.Collection.List(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	names, err := h.deps.Collection.SeriesNames(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	items := make([]comicDTO, 0, len(comics))
	for _, c := range comics {
		items = append(items, toComicDTO(c))
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, collectionResponse{Items: items, Series: names})
}

func (h *handlers) checkCollection(w http.ResponseWriter, r *http.Request) {
	query, problem := parseIssueQuery(r.URL.Query(), false)
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	comic, err := h.deps.Collection.Find(r.Context(), query.Series, query.Number)
	switch {
	case errors.Is(err, collection.ErrNotFound):
		writeJSON(w, http.StatusOK, checkResponse{Owned: false})
	case err != nil:
		h.internalError(w, r, err)
	default:
		dto := toComicDTO(comic)
		writeJSON(w, http.StatusOK, checkResponse{Owned: true, Item: &dto})
	}
}

func (h *handlers) addComic(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, photoTooLargeMessage)
			return
		}
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	series, seriesOK := bounded(req.Series, maxSeriesRunes)
	number, numberOK := bounded(req.Number, maxNumberRunes)
	switch {
	case series == "":
		writeError(w, http.StatusBadRequest, "Series is required")
		return
	case number == "":
		writeError(w, http.StatusBadRequest, "Issue number is required")
		return
	case !seriesOK || !numberOK:
		writeError(w, http.StatusBadRequest, "Series and issue number must be shorter")
		return
	}

	image, err := photo.ParseDataURL(req.Photo)
	switch {
	case errors.Is(err, photo.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, photoTooLargeMessage)
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, photoRequiredMessage)
		return
	}

	comic, err := h.deps.Collection.Add(r.Context(), collection.NewComic{
		Series:    series,
		Number:    number,
		PhotoData: image.Data,
		PhotoExt:  image.Extension,
		Match:     sanitizeMatch(req.Match),
	})
	var duplicate *collection.DuplicateError
	switch {
	case errors.As(err, &duplicate):
		writeJSON(w, http.StatusConflict, conflictResponse{
			Error: "This issue is already in the collection",
			Item:  toComicDTO(duplicate.Existing),
		})
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusCreated, addResponse{Item: toComicDTO(comic)})
	}
}

func (h *handlers) removeComic(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid id")
		return
	}
	err := h.deps.Collection.Remove(r.Context(), id)
	switch {
	case errors.Is(err, collection.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

var photoTypes = map[string]string{
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

func (h *handlers) comicPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	file, err := h.deps.Collection.PhotoPath(r.Context(), id)
	if errors.Is(err, collection.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) { //nolint:gosec // G703: the path comes from the collection store, never from request input
		writeError(w, http.StatusNotFound, "Not found")
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}

	if contentType, ok := photoTypes[filepath.Ext(file)]; ok {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, file) //nolint:gosec // G703: the path comes from the collection store, never from request input
}

func (h *handlers) lookupIssue(w http.ResponseWriter, r *http.Request) {
	query, problem := parseIssueQuery(r.URL.Query(), true)
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}

	result := h.deps.Lookup.Lookup(r.Context(), query)
	candidates := result.Candidates
	if candidates == nil {
		candidates = []catalog.Candidate{}
	}
	sources := make(map[string]sourceStatus, len(result.Statuses))
	for name, status := range result.Statuses {
		sources[name] = sourceStatus{OK: status.OK, Count: status.Count, Message: status.Message}
	}
	writeJSON(w, http.StatusOK, lookupResponse{
		Series:     query.Series,
		Number:     query.Number,
		Year:       query.Year,
		Candidates: candidates,
		Sources:    sources,
	})
}

func (h *handlers) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.deps.Logger.ErrorContext(r.Context(), "request failed",
		"method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "Something went wrong on the server")
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "Not found")
}
