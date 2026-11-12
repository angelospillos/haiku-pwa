import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createCollection, DuplicateComicError, numberKey, seriesKey } from '../../server/collection.js';
import { openCollectionDb } from '../../server/db.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const tmp = path.join(root, 'tests/.tmp/collection-unit');
const photo = { bytes: Buffer.from('fake-jpeg'), extension: 'jpg' };

describe('series and number normalisation', () => {
  it('ignores case, accents and surrounding spaces in series names', () => {
    assert.equal(seriesKey('  Astérix  '), 'asterix');
    assert.equal(seriesKey('ASTERIX'), seriesKey('asterix'));
  });

  it('treats leading zeros and spacing in issue numbers as the same issue', () => {
    assert.equal(numberKey('05'), '5');
    assert.equal(numberKey(' 5 '), '5');
    assert.equal(numberKey('12a'), '12A');
    assert.equal(numberKey('2001-03'), '2001-03');
  });
});

describe('collection repository', () => {
  let db;
  let collection;

  before(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
    db = openCollectionDb(path.join(tmp, 'collection.sqlite'));
    collection = createCollection(db, path.join(tmp, 'photos'));
  });

  after(() => {
    db.close();
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it('adds an issue, finds it under a different spelling and removes it with its photo', () => {
    const item = collection.add({ series: 'Asterix', number: '5', photo, match: null });
    assert.equal(item.series, 'Asterix');
    assert.equal(item.photoUrl, `/api/comics/${item.id}/photo`);
    assert.equal(collection.find('ASTERIX ', '05')?.id, item.id);

    const photoFile = collection.photoPath(item.id);
    assert.ok(fs.existsSync(photoFile));
    assert.equal(collection.remove(item.id), true);
    assert.equal(collection.find('Asterix', '5'), null);
    assert.equal(fs.existsSync(photoFile), false);
  });

  it('refuses a second copy of the same issue and returns the existing one', () => {
    const first = collection.add({ series: 'Asterix', number: '7', photo, match: null });
    assert.throws(
      () => collection.add({ series: 'asterix', number: '07', photo, match: null }),
      (error) => error instanceof DuplicateComicError && error.existing.id === first.id,
    );
    collection.remove(first.id);
  });

  it('keeps the catalogue details that were matched when the issue was saved', () => {
    const item = collection.add({
      series: 'Topolino',
      number: '3401',
      photo,
      match: {
        source: 'inducks',
        sourceId: 'it/TL 3401',
        sourceUrl: 'https://inducks.org/issue.php?c=it%2FTL+3401',
        seriesTitle: 'Topolino (libretto) · Italy',
        title: '',
        publicationDate: '2021-01-27',
        coverUrl: null,
      },
    });
    assert.equal(item.source, 'inducks');
    assert.equal(item.seriesTitle, 'Topolino (libretto) · Italy');
    assert.equal(item.publicationDate, '2021-01-27');
    collection.remove(item.id);
  });

  it('lists issues in series and numeric order', () => {
    const ids = ['10', '2', '1'].map((number) => collection.add({ series: 'Zeta', number, photo, match: null }).id);
    const numbers = collection.list().map((item) => item.number);
    assert.deepEqual(numbers, ['1', '2', '10']);
    ids.forEach((id) => collection.remove(id));
  });
});
