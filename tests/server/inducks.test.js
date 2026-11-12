import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { importInducks } from '../../server/metadata/inducks-import.js';
import { createInducksSource, openInducksDb } from '../../server/metadata/inducks.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const tmp = path.join(root, 'tests/.tmp/inducks-unit');
const dbPath = path.join(tmp, 'inducks.sqlite');

describe('Inducks import and search', () => {
  let source;

  before(async () => {
    fs.rmSync(tmp, { recursive: true, force: true });
    const counts = await importInducks({ dir: path.join(root, 'tests/fixtures/inducks/isv'), dbPath });
    assert.equal(counts.issues, 4);
    source = createInducksSource(openInducksDb(dbPath));
  });

  after(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it('reports when the data was synced', () => {
    assert.equal(source.available, true);
    assert.match(source.syncedAt, /^\d{4}-\d{2}-\d{2}T/);
  });

  it('finds an Italian Topolino issue by publication name and issue number', () => {
    const [match] = source.searchIssues({ series: 'Topolino', number: '3401' });
    assert.deepEqual(match, {
      source: 'inducks',
      sourceId: 'it/TL 3401',
      sourceUrl: 'https://inducks.org/issue.php?c=it%2FTL+3401',
      seriesTitle: 'Topolino (libretto) · Italy',
      title: '',
      publicationDate: '2021-01-27',
      coverUrl: null,
    });
  });

  it('returns nothing for an issue that is not in the index', () => {
    assert.deepEqual(source.searchIssues({ series: 'Topolino', number: '1' }), []);
    assert.deepEqual(source.searchIssues({ series: 'Asterix', number: '5' }), []);
  });

  it('matches the country-coded issue numbers that contain dashes', () => {
    const [match] = source.searchIssues({ series: 'Micky Maus', number: '2001-03' });
    assert.equal(match.sourceId, 'de/MM2001-03');
    assert.equal(match.seriesTitle, 'Micky Maus · Germany');
  });

  it('treats LIKE wildcards in the series name as plain text', () => {
    assert.deepEqual(source.searchIssues({ series: 'Top%', number: '3401' }), []);
  });

  it('is unavailable until the Inducks data has been synced', () => {
    const missing = createInducksSource(null);
    assert.equal(missing.available, false);
    assert.throws(() => missing.searchIssues({ series: 'Topolino', number: '3401' }), /not been synced/);
  });
});
