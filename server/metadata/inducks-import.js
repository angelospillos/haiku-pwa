import fs from 'node:fs';
import path from 'node:path';
import readline from 'node:readline';
import { DatabaseSync } from 'node:sqlite';

export const INDUCKS_MEMBERS = [
  'inducks_issue.isv',
  'inducks_publication.isv',
  'inducks_publicationname.isv',
  'inducks_countryname.isv',
];

const SCHEMA = `
CREATE TABLE country (countrycode TEXT PRIMARY KEY, countryname TEXT NOT NULL);
CREATE TABLE publication (publicationcode TEXT PRIMARY KEY, countrycode TEXT, title TEXT NOT NULL);
CREATE TABLE publication_name (publicationcode TEXT NOT NULL, publicationname TEXT NOT NULL);
CREATE TABLE issue (
  issuecode TEXT PRIMARY KEY,
  publicationcode TEXT NOT NULL,
  issuenumber TEXT NOT NULL,
  title TEXT,
  oldestdate TEXT
);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`;

const INDEXES = `
CREATE INDEX issue_number ON issue (issuenumber);
CREATE INDEX publication_name_code ON publication_name (publicationcode);
`;

async function* readIsv(file) {
  if (!fs.existsSync(file)) throw new Error(`Missing Inducks file: ${file}`);
  const lines = readline.createInterface({
    input: fs.createReadStream(file, { encoding: 'utf8' }),
    crlfDelay: Infinity,
  });
  let isHeader = true;
  for await (const line of lines) {
    if (isHeader) {
      isHeader = false;
      continue;
    }
    if (line) yield line.split('^');
  }
}

export async function importInducks({ dir, dbPath }) {
  const tmpPath = `${dbPath}.tmp`;
  fs.mkdirSync(path.dirname(dbPath), { recursive: true });
  fs.rmSync(tmpPath, { force: true });

  const db = new DatabaseSync(tmpPath);
  const counts = { countries: 0, publications: 0, publicationNames: 0, issues: 0 };
  try {
    db.exec(SCHEMA);
    db.exec('BEGIN');

    const insertCountry = db.prepare('INSERT OR IGNORE INTO country VALUES (?, ?)');
    for await (const [countrycode, languagecode, countryname] of readIsv(path.join(dir, 'inducks_countryname.isv'))) {
      if (languagecode === 'en' && countrycode && countryname) {
        insertCountry.run(countrycode, countryname);
        counts.countries++;
      }
    }

    const insertPublication = db.prepare('INSERT OR IGNORE INTO publication VALUES (?, ?, ?)');
    for await (const [publicationcode, countrycode, , title] of readIsv(path.join(dir, 'inducks_publication.isv'))) {
      if (publicationcode && title) {
        insertPublication.run(publicationcode, countrycode, title);
        counts.publications++;
      }
    }

    const insertPublicationName = db.prepare('INSERT INTO publication_name VALUES (?, ?)');
    for await (const [publicationcode, publicationname] of readIsv(path.join(dir, 'inducks_publicationname.isv'))) {
      if (publicationcode && publicationname) {
        insertPublicationName.run(publicationcode, publicationname);
        counts.publicationNames++;
      }
    }

    const insertIssue = db.prepare('INSERT OR IGNORE INTO issue VALUES (?, ?, ?, ?, ?)');
    for await (const fields of readIsv(path.join(dir, 'inducks_issue.isv'))) {
      if (fields.length < 11) continue;
      const [issuecode, , publicationcode, issuenumber, title, , , , , , oldestdate] = fields;
      if (!issuecode || !publicationcode || !issuenumber) continue;
      insertIssue.run(issuecode, publicationcode, issuenumber, title || null, oldestdate && !oldestdate.startsWith('9999') ? oldestdate : null);
      counts.issues++;
    }

    db.exec('COMMIT');
    db.exec(INDEXES);
    const insertMeta = db.prepare('INSERT INTO meta VALUES (?, ?)');
    insertMeta.run('synced_at', new Date().toISOString());
    insertMeta.run('issues', String(counts.issues));
    insertMeta.run('publications', String(counts.publications));
    db.close();
    fs.renameSync(tmpPath, dbPath);
    return counts;
  } catch (error) {
    try {
      db.exec('ROLLBACK');
    } catch {
      // no transaction open
    }
    try {
      db.close();
    } catch {
      // already closed
    }
    fs.rmSync(tmpPath, { force: true });
    throw error;
  }
}
