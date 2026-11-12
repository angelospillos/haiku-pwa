CREATE TABLE IF NOT EXISTS comics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  series TEXT NOT NULL,
  series_key TEXT NOT NULL,
  number TEXT NOT NULL,
  number_key TEXT NOT NULL,
  source TEXT,
  source_id TEXT,
  source_url TEXT,
  series_title TEXT,
  title TEXT,
  publication_date TEXT,
  cover_url TEXT,
  photo_file TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (series_key, number_key)
);

CREATE TABLE IF NOT EXISTS http_cache (
  cache_key TEXT PRIMARY KEY,
  payload BLOB NOT NULL,
  fetched_at INTEGER NOT NULL
);
