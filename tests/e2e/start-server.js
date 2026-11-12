// Starts the real app for the end-to-end tests: a fresh data directory, the small
// Inducks fixture imported into its own database, and the stub comics.org API
// configured through COMICS_ORG_API_URL (see playwright.config.js).

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { importInducks } from '../../server/metadata/inducks-import.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

if (!process.env.DATA_DIR || !process.env.INDUCKS_DB_PATH) {
  throw new Error('DATA_DIR and INDUCKS_DB_PATH must be set');
}

fs.rmSync(process.env.DATA_DIR, { recursive: true, force: true });
fs.rmSync(process.env.INDUCKS_DB_PATH, { force: true });
await importInducks({
  dir: path.join(root, 'tests/fixtures/inducks/isv'),
  dbPath: process.env.INDUCKS_DB_PATH,
});

await import('../../server/index.js');
