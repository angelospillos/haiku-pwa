import fs from 'node:fs';
import { DatabaseSync } from 'node:sqlite';
import { cleanText } from './util.js';

const MAX_CANDIDATES = 20;

function escapeLike(value) {
  return value.replace(/[\\%_]/g, (char) => `\\${char}`);
}

function validDate(value) {
  return value && /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : '';
}

export function openInducksDb(file) {
  if (!fs.existsSync(file)) return null;
  return new DatabaseSync(file, { readOnly: true });
}

export function createInducksSource(db) {
  const syncedAt = db ? db.prepare("SELECT value FROM meta WHERE key = 'synced_at'").get()?.value : null;

  const searchStmt = db
    ? db.prepare(`
        SELECT i.issuecode, i.title AS issue_title, i.oldestdate,
               p.title AS publication_title, c.countryname
        FROM issue i
        JOIN publication p ON p.publicationcode = i.publicationcode
        LEFT JOIN country c ON c.countrycode = p.countrycode
        WHERE i.issuenumber = ?
          AND (
            p.title LIKE ? ESCAPE '\\'
            OR p.publicationcode IN (
              SELECT publicationcode FROM publication_name WHERE publicationname LIKE ? ESCAPE '\\'
            )
          )
        ORDER BY (p.title LIKE ? ESCAPE '\\') DESC, p.title, i.oldestdate
        LIMIT ?
      `)
    : null;

  return {
    get available() {
      return Boolean(db);
    },

    syncedAt,

    searchIssues({ series, number }) {
      if (!searchStmt) throw new Error('Inducks data has not been synced yet (run npm run sync:inducks)');
      const term = escapeLike(series);
      const rows = searchStmt.all(number, `%${term}%`, `%${term}%`, `${term}%`, MAX_CANDIDATES);
      return rows.map((row) => ({
        source: 'inducks',
        sourceId: row.issuecode,
        sourceUrl: `https://inducks.org/issue.php?${new URLSearchParams({ c: row.issuecode })}`,
        seriesTitle: cleanText([row.publication_title, row.countryname].filter(Boolean).join(' · '), 200),
        title: cleanText(row.issue_title ?? '', 200),
        publicationDate: validDate(row.oldestdate),
        coverUrl: null,
      }));
    },
  };
}
