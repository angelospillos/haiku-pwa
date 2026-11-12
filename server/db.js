import fs from 'node:fs';
import path from 'node:path';
import { DatabaseSync } from 'node:sqlite';

const SCHEMA = `
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
  payload TEXT NOT NULL,
  fetched_at INTEGER NOT NULL
);
`;

export function openCollectionDb(file) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const db = new DatabaseSync(file);
  db.exec('PRAGMA journal_mode = WAL;');
  db.exec(SCHEMA);
  return db;
}
