// Package comicsorg looks issues up in the Grand Comics Database through its
// public API (https://www.comics.org/api/schema/redoc). No account is needed.
package comicsorg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	maxCandidates     = 8
	detailConcurrency = 3
	cacheTTL          = 7 * 24 * time.Hour
	maxResponseBytes  = 4 << 20
	defaultTimeout    = 10 * time.Second
	// The service is run by volunteers, so requests are spaced out and bursts
	// are capped well below what the API would otherwise accept.
	requestsPerSecond = 1
	requestBurst      = 3
	userAgent         = "haiku-pwa/0.1 (personal comic collection; +https://github.com/angelospillos/haiku-pwa)"
	issuePageURL      = "https://www.comics.org/issue/%s/"
)

var errNotFound = errors.New("not found")

var issueIDPattern = regexp.MustCompile(`/issue/(\d+)/`)

// Cache stores response bodies so repeat lookups do not reach the API again.
type Cache interface {
	Get(ctx context.Context, key string, maxAge time.Duration) ([]byte, bool, error)
	Put(ctx context.Context, key string, payload []byte) error
}

// Options configures a Client. Zero values select sensible defaults.
type Options struct {
	BaseURL    string
	HTTPClient *http.Client
	Cache      Cache
	Logger     *slog.Logger
}

// Client searches the comics.org API.
type Client struct {
	baseURL string
	http    *http.Client
	cache   Cache
	logger  *slog.Logger
	limiter *rate.Limiter
}

// New returns a Client for the API rooted at opts.BaseURL.
func New(opts Options) *Client {
	c := &Client{
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		http:    opts.HTTPClient,
		cache:   opts.Cache,
		logger:  opts.Logger,
		limiter: rate.NewLimiter(requestsPerSecond, requestBurst),
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: defaultTimeout}
	}
	if c.logger == nil {
		c.logger = slog.Default()
	}
	return c
}

type searchResponse struct {
	Results []summary `json:"results"`
}

type summary struct {
	APIURL          string  `json:"api_url"`
	SeriesName      string  `json:"series_name"`
	Descriptor      string  `json:"descriptor"`
	PublicationDate string  `json:"publication_date"`
	VariantOf       *string `json:"variant_of"`
}

type issueDetail struct {
	Title           string `json:"title"`
	PublicationDate string `json:"publication_date"`
	Cover           string `json:"cover"`
}

// Search returns the issues matching q. Variant covers are skipped. Details
// such as cover art are fetched for the first few matches; a failed detail
// request only drops that match's extra information.
func (c *Client) Search(ctx context.Context, q catalog.Query) ([]catalog.Candidate, error) {
	var list searchResponse
	err := c.getJSON(ctx, c.searchURL(q), &list)
	switch {
	case errors.Is(err, errNotFound):
		list = searchResponse{}
	case err != nil:
		return nil, err
	}

	summaries := make([]summary, 0, maxCandidates)
	for _, s := range list.Results {
		if s.VariantOf != nil {
			continue
		}
		summaries = append(summaries, s)
		if len(summaries) == maxCandidates {
			break
		}
	}

	details := make([]*issueDetail, len(summaries))
	var group errgroup.Group
	group.SetLimit(detailConcurrency)
	for i, s := range summaries {
		group.Go(func() error {
			if s.APIURL == "" {
				return nil
			}
			var detail issueDetail
			if err := c.getJSON(ctx, s.APIURL, &detail); err != nil {
				c.logger.Warn("comics.org issue detail unavailable", "url", s.APIURL, "error", err)
				return nil
			}
			details[i] = &detail
			return nil
		})
	}
	_ = group.Wait()

	candidates := make([]catalog.Candidate, 0, len(summaries))
	for i, s := range summaries {
		candidate := toCandidate(s, details[i])
		if candidate.SourceID != "" {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func (c *Client) searchURL(q catalog.Query) string {
	u := c.baseURL + "/series/name/" + url.PathEscape(q.Series) + "/issue/" + url.PathEscape(q.Number) + "/"
	if q.Year != "" {
		u += "year/" + url.PathEscape(q.Year) + "/"
	}
	return u + "?format=json"
}

func toCandidate(s summary, detail *issueDetail) catalog.Candidate {
	candidate := catalog.Candidate{
		Source:          catalog.SourceComicsOrg,
		SeriesTitle:     catalog.CleanText(s.SeriesName, 200),
		Title:           catalog.CleanText(s.Descriptor, 200),
		PublicationDate: catalog.CleanText(s.PublicationDate, 40),
	}
	if m := issueIDPattern.FindStringSubmatch(s.APIURL); m != nil {
		candidate.SourceID = m[1]
		candidate.SourceURL = fmt.Sprintf(issuePageURL, m[1])
	}
	if detail != nil {
		if title := catalog.CleanText(detail.Title, 200); title != "" {
			candidate.Title = title
		}
		if date := catalog.CleanText(detail.PublicationDate, 40); date != "" {
			candidate.PublicationDate = date
		}
		if isHTTPURL(detail.Cover) {
			candidate.CoverURL = detail.Cover
		}
	}
	return candidate
}

func (c *Client) getJSON(ctx context.Context, rawURL string, dst any) error {
	key := "comicsorg:" + rawURL
	if c.cache != nil {
		payload, ok, err := c.cache.Get(ctx, key, cacheTTL)
		switch {
		case err != nil:
			c.logger.Warn("comics.org cache read failed", "error", err)
		case ok:
			if err := json.Unmarshal(payload, dst); err != nil {
				return unavailable("returned cached data that could not be parsed")
			}
			return nil
		}
	}

	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build comics.org request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return unavailable("could not be reached")
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return unavailable("is rate-limiting requests right now; try again in a few minutes")
	case resp.StatusCode != http.StatusOK:
		return unavailable(fmt.Sprintf("responded with HTTP %d", resp.StatusCode))
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return unavailable("could not be read")
	}
	if err := json.Unmarshal(payload, dst); err != nil {
		return unavailable("returned data that could not be parsed")
	}
	if c.cache != nil {
		if err := c.cache.Put(ctx, key, payload); err != nil {
			c.logger.Warn("comics.org cache write failed", "error", err)
		}
	}
	return nil
}

func unavailable(reason string) error {
	return &catalog.UnavailableError{Source: catalog.SourceComicsOrg, Reason: reason}
}

func isHTTPURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
