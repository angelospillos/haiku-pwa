package config

import (
	"path/filepath"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.ComicsOrgAPIURL != "https://www.comics.org/api" {
		t.Errorf("ComicsOrgAPIURL = %q", cfg.ComicsOrgAPIURL)
	}
	if !filepath.IsAbs(cfg.CollectionDBPath) {
		t.Errorf("CollectionDBPath %q is not absolute", cfg.CollectionDBPath)
	}
	if filepath.Base(cfg.InducksDBPath) != "inducks.sqlite" {
		t.Errorf("InducksDBPath = %q", cfg.InducksDBPath)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want text", cfg.LogFormat)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"PORT":               "8080",
		"DATA_DIR":           "/srv/comics",
		"INDUCKS_DB_PATH":    "/srv/index/inducks.sqlite",
		"COMICS_ORG_API_URL": "https://example.test/api/",
		"LOG_FORMAT":         "json",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if cfg.CollectionDBPath != "/srv/comics/collection.sqlite" {
		t.Errorf("CollectionDBPath = %q", cfg.CollectionDBPath)
	}
	if cfg.PhotosDir != "/srv/comics/photos" {
		t.Errorf("PhotosDir = %q", cfg.PhotosDir)
	}
	if cfg.InducksDBPath != "/srv/index/inducks.sqlite" {
		t.Errorf("InducksDBPath = %q", cfg.InducksDBPath)
	}
	if cfg.ComicsOrgAPIURL != "https://example.test/api" {
		t.Errorf("ComicsOrgAPIURL = %q, want trailing slash trimmed", cfg.ComicsOrgAPIURL)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q", cfg.LogFormat)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]map[string]string{
		"port not a number":   {"PORT": "http"},
		"port out of range":   {"PORT": "70000"},
		"unknown log format":  {"LOG_FORMAT": "yaml"},
		"relative api url":    {"COMICS_ORG_API_URL": "/api"},
		"non-http api scheme": {"COMICS_ORG_API_URL": "ftp://example.test/api"},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(values)); err == nil {
				t.Fatal("Load() succeeded, want an error")
			}
		})
	}
}
