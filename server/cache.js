export function createHttpCache(db) {
  const selectStmt = db.prepare('SELECT payload, fetched_at FROM http_cache WHERE cache_key = ?');
  const upsertStmt = db.prepare(`
    INSERT INTO http_cache (cache_key, payload, fetched_at) VALUES (?, ?, ?)
    ON CONFLICT (cache_key) DO UPDATE SET payload = excluded.payload, fetched_at = excluded.fetched_at
  `);

  return {
    get(key, maxAgeMs) {
      const row = selectStmt.get(key);
      if (!row || Date.now() - row.fetched_at > maxAgeMs) return undefined;
      return JSON.parse(row.payload);
    },

    set(key, value) {
      upsertStmt.run(key, JSON.stringify(value), Date.now());
    },
  };
}
