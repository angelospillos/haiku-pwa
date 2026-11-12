// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultPort            = 3000
	defaultDataDir         = "data"
	defaultWebDir          = "dist"
	defaultComicsOrgAPIURL = "https://www.comics.org/api"
)

// Config holds every setting the server and tools read at startup.
type Config struct {
	Port             int
	DataDir          string
	WebDir           string
	CollectionDBPath string
	PhotosDir        string
	InducksDBPath    string
	ComicsOrgAPIURL  string
	LogFormat        string
}

// Load reads the configuration using getenv, which is os.Getenv in production
// and a map lookup in tests. Relative paths are resolved against the working
// directory so the process behaves the same wherever it is started from.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:            defaultPort,
		DataDir:         defaultDataDir,
		WebDir:          defaultWebDir,
		ComicsOrgAPIURL: defaultComicsOrgAPIURL,
		LogFormat:       "text",
	}

	if v := getenv("PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number between 1 and 65535, got %q", v)
		}
		cfg.Port = port
	}
	if v := getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := getenv("WEB_DIR"); v != "" {
		cfg.WebDir = v
	}
	if v := getenv("COMICS_ORG_API_URL"); v != "" {
		cfg.ComicsOrgAPIURL = v
	}
	if v := getenv("LOG_FORMAT"); v != "" {
		if v != "text" && v != "json" {
			return Config{}, fmt.Errorf("LOG_FORMAT must be text or json, got %q", v)
		}
		cfg.LogFormat = v
	}

	var err error
	if cfg.DataDir, err = filepath.Abs(cfg.DataDir); err != nil {
		return Config{}, fmt.Errorf("DATA_DIR: %w", err)
	}
	if cfg.WebDir, err = filepath.Abs(cfg.WebDir); err != nil {
		return Config{}, fmt.Errorf("WEB_DIR: %w", err)
	}

	cfg.CollectionDBPath = filepath.Join(cfg.DataDir, "collection.sqlite")
	cfg.PhotosDir = filepath.Join(cfg.DataDir, "photos")
	cfg.InducksDBPath = filepath.Join(cfg.DataDir, "inducks.sqlite")
	if v := getenv("INDUCKS_DB_PATH"); v != "" {
		if cfg.InducksDBPath, err = filepath.Abs(v); err != nil {
			return Config{}, fmt.Errorf("INDUCKS_DB_PATH: %w", err)
		}
	}

	cfg.ComicsOrgAPIURL = strings.TrimRight(cfg.ComicsOrgAPIURL, "/")
	u, err := url.Parse(cfg.ComicsOrgAPIURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, errors.New("COMICS_ORG_API_URL must be an absolute http or https URL")
	}

	return cfg, nil
}
