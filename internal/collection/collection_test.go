package collection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/angelospillos/haiku-pwa/internal/catalog"
	"github.com/angelospillos/haiku-pwa/internal/store"
)

func TestSeriesKey(t *testing.T) {
	tests := map[string]string{
		"Asterix":             "asterix",
		"  ASTÉRIX  ":         "asterix",
		"Astérix\u00a0 x":     "asterix x",
		"Topolino (libretto)": "topolino (libretto)",
	}
	for in, want := range tests {
		if got := SeriesKey(in); got != want {
			t.Errorf("SeriesKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumberKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"5", "5"},
		{"05", "5"},
		{" 5 ", "5"},
		{"00", "0"},
		{"0A", "0A"},
		{"12a", "12A"},
		{"2001-03", "2001-03"},
	}
	for _, tt := range tests {
		if got := NumberKey(tt.in); got != tt.want {
			t.Errorf("NumberKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func openStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.OpenCollection(context.Background(), filepath.Join(dir, "collection.sqlite"))
	if err != nil {
		t.Fatalf("OpenCollection() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	photos := filepath.Join(dir, "photos")
	return NewStore(db, photos), photos
}

func photo() NewComic {
	return NewComic{Series: "Asterix", Number: "5", PhotoData: []byte("jpeg-bytes"), PhotoExt: "jpg"}
}

func TestAddFindAndRemove(t *testing.T) {
	ctx := context.Background()
	s, photosDir := openStore(t)

	comic, err := s.Add(ctx, photo())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if comic.Series != "Asterix" || comic.Number != "5" {
		t.Errorf("saved %q #%q", comic.Series, comic.Number)
	}

	found, err := s.Find(ctx, "ASTERIX ", "05")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found.ID != comic.ID {
		t.Errorf("Find() id = %d, want %d", found.ID, comic.ID)
	}

	photoPath, err := s.PhotoPath(ctx, comic.ID)
	if err != nil {
		t.Fatalf("PhotoPath() error = %v", err)
	}
	if filepath.Dir(photoPath) != photosDir {
		t.Errorf("photo stored in %q, want %q", filepath.Dir(photoPath), photosDir)
	}
	if _, err := os.Stat(photoPath); err != nil {
		t.Fatalf("photo file missing: %v", err)
	}

	if err := s.Remove(ctx, comic.ID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := s.Find(ctx, "Asterix", "5"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find() after Remove error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(photoPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("photo still exists after Remove: %v", err)
	}
	if err := s.Remove(ctx, comic.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Remove() error = %v, want ErrNotFound", err)
	}
}

func TestAddRejectsDuplicateAndReturnsExisting(t *testing.T) {
	ctx := context.Background()
	s, _ := openStore(t)

	first, err := s.Add(ctx, photo())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	dup := photo()
	dup.Series = "asterix"
	dup.Number = "05"
	_, err = s.Add(ctx, dup)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Add() duplicate error = %v, want ErrDuplicate", err)
	}
	var dupErr *DuplicateError
	if !errors.As(err, &dupErr) || dupErr.Existing.ID != first.ID {
		t.Errorf("DuplicateError.Existing = %+v, want id %d", dupErr, first.ID)
	}
}

func TestAddStoresCatalogueDetails(t *testing.T) {
	ctx := context.Background()
	s, _ := openStore(t)

	in := NewComic{
		Series:    "Topolino",
		Number:    "3401",
		PhotoData: []byte("bytes"),
		PhotoExt:  "jpg",
		Match: &catalog.Candidate{
			Source:          catalog.SourceInducks,
			SourceID:        "it/TL 3401",
			SourceURL:       "https://inducks.org/issue.php?c=it%2FTL+3401",
			SeriesTitle:     "Topolino (libretto) · Italy",
			PublicationDate: "2021-01-27",
		},
	}
	comic, err := s.Add(ctx, in)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if comic.Source != catalog.SourceInducks || comic.PublicationDate != "2021-01-27" {
		t.Errorf("saved details = %+v", comic)
	}
}

func TestAddWithoutMatchLeavesDetailsEmpty(t *testing.T) {
	ctx := context.Background()
	s, _ := openStore(t)

	comic, err := s.Add(ctx, photo())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if comic.Source != "" || comic.SourceURL != "" || comic.Title != "" {
		t.Errorf("expected no catalogue details, got %+v", comic)
	}
}

func TestListOrdersBySeriesThenNumericIssue(t *testing.T) {
	ctx := context.Background()
	s, _ := openStore(t)

	for _, number := range []string{"10", "2", "1"} {
		in := photo()
		in.Series = "Zeta"
		in.Number = number
		if _, err := s.Add(ctx, in); err != nil {
			t.Fatalf("Add(%s) error = %v", number, err)
		}
	}
	comics, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var got []string
	for _, c := range comics {
		got = append(got, c.Number)
	}
	want := []string{"1", "2", "10"}
	if len(got) != len(want) {
		t.Fatalf("List() numbers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() numbers = %v, want %v", got, want)
		}
	}
}
