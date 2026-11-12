import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createApp } from '../../server/app.js';
import { loadConfig } from '../../server/config.js';
import { openCollectionDb } from '../../server/db.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const tmp = path.join(root, 'tests/.tmp/api-unit');
const JPEG_DATA_URL = `data:image/jpeg;base64,${Buffer.from('not really a jpeg but bytes').toString('base64')}`;
const json = (body, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

function fakeComicsOrg() {
  const calls = [];
  const fetchImpl = async (url) => {
    calls.push(url);
    if (url.includes('/series/name/Asterix/issue/5/')) {
      return json({
        count: 2,
        results: [
          {
            api_url: 'https://gcd.test/api/issue/10/?format=json',
            series_name: 'Asterix (1961 series)',
            descriptor: '5 - Le tour de Gaule',
            publication_date: '1965',
            variant_of: null,
          },
          {
            api_url: 'https://gcd.test/api/issue/11/?format=json',
            series_name: 'Asterix (1961 series)',
            descriptor: '5 - Variant',
            publication_date: '1965',
            variant_of: 'https://gcd.test/api/issue/10/?format=json',
          },
        ],
      });
    }
    if (url.includes('/issue/10/')) {
      return json({ title: 'Le tour de Gaule', publication_date: '1965', cover: 'https://files.test/10.jpg' });
    }
    if (url.includes('/series/name/Broken/')) return new Response('', { status: 503 });
    return json({ count: 0, results: [] });
  };
  return { calls, fetchImpl };
}

describe('HTTP API', () => {
  let server;
  let base;
  let db;
  let comicsOrg;

  before(async () => {
    fs.rmSync(tmp, { recursive: true, force: true });
    const config = loadConfig({ DATA_DIR: tmp, COMICS_ORG_API_URL: 'https://gcd.test/api' });
    db = openCollectionDb(config.collectionDbPath);
    comicsOrg = fakeComicsOrg();
    const app = createApp({ config, db, inducksDb: null, fetchImpl: comicsOrg.fetchImpl });
    server = await new Promise((resolve) => {
      const listening = app.listen(0, '127.0.0.1', () => resolve(listening));
    });
    base = `http://127.0.0.1:${server.address().port}`;
  });

  after(async () => {
    await new Promise((resolve) => server.close(resolve));
    db.close();
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  const post = (body) =>
    fetch(`${base}/api/collection`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

  it('reports health', async () => {
    const response = await fetch(`${base}/api/health`);
    assert.deepEqual(await response.json(), { ok: true, inducks: false });
  });

  it('merges comics.org matches with cover art and skips variant covers', async () => {
    const body = await (await fetch(`${base}/api/lookup?series=Asterix&number=5`)).json();
    assert.equal(body.candidates.length, 1);
    assert.deepEqual(body.candidates[0], {
      source: 'comics.org',
      sourceId: '10',
      sourceUrl: 'https://www.comics.org/issue/10/',
      seriesTitle: 'Asterix (1961 series)',
      title: 'Le tour de Gaule',
      publicationDate: '1965',
      coverUrl: 'https://files.test/10.jpg',
    });
    assert.equal(body.sources['comics.org'].ok, true);
    assert.equal(body.sources.inducks.ok, false);
    assert.match(body.sources.inducks.message, /not been synced/);
  });

  it('caches comics.org responses so repeat lookups do not hit the API again', async () => {
    const before = comicsOrg.calls.length;
    await (await fetch(`${base}/api/lookup?series=Asterix&number=5`)).json();
    assert.equal(comicsOrg.calls.length, before);
  });

  it('keeps going when comics.org fails and says which source was unavailable', async () => {
    const body = await (await fetch(`${base}/api/lookup?series=Broken&number=1`)).json();
    assert.deepEqual(body.candidates, []);
    assert.equal(body.sources['comics.org'].ok, false);
    assert.equal(body.sources['comics.org'].message, 'comics.org responded with HTTP 503');
  });

  it('validates lookup parameters', async () => {
    assert.equal((await fetch(`${base}/api/lookup?number=5`)).status, 400);
    assert.equal((await fetch(`${base}/api/lookup?series=Asterix`)).status, 400);
    assert.equal((await fetch(`${base}/api/lookup?series=Asterix&number=5&year=65`)).status, 400);
  });

  it('rejects an entry without a series, number or photo', async () => {
    assert.equal((await post({ number: '5', photo: JPEG_DATA_URL })).status, 400);
    assert.equal((await post({ series: 'Asterix', photo: JPEG_DATA_URL })).status, 400);
    const noPhoto = await post({ series: 'Asterix', number: '5' });
    assert.equal(noPhoto.status, 400);
    assert.match((await noPhoto.json()).error, /photo/);
    assert.equal((await post({ series: 'Asterix', number: '5', photo: 'data:text/plain;base64,aGk=' })).status, 400);
  });

  it('saves an issue with its photo and refuses a duplicate', async () => {
    const created = await post({
      series: 'Asterix',
      number: '5',
      photo: JPEG_DATA_URL,
      match: {
        source: 'comics.org',
        sourceId: '10',
        sourceUrl: 'https://www.comics.org/issue/10/',
        seriesTitle: 'Asterix (1961 series)',
        title: 'Le tour de Gaule',
        publicationDate: '1965',
        coverUrl: 'https://files.test/10.jpg',
      },
    });
    assert.equal(created.status, 201);
    const { item } = await created.json();
    assert.equal(item.title, 'Le tour de Gaule');

    const photo = await fetch(`${base}${item.photoUrl}`);
    assert.equal(photo.status, 200);
    assert.equal(photo.headers.get('content-type'), 'image/jpeg');

    const duplicate = await post({ series: 'asterix', number: '05', photo: JPEG_DATA_URL });
    assert.equal(duplicate.status, 409);
    assert.equal((await duplicate.json()).item.id, item.id);

    const check = await (await fetch(`${base}/api/collection/check?series=ASTERIX&number=5`)).json();
    assert.equal(check.owned, true);
  });

  it('ignores match details it cannot trust', async () => {
    const created = await post({
      series: 'Odd',
      number: '1',
      photo: JPEG_DATA_URL,
      match: { source: 'some-other-site', sourceUrl: 'javascript:alert(1)', coverUrl: 'ftp://files.test/x.jpg' },
    });
    const { item } = await created.json();
    assert.equal(item.source, null);
    assert.equal(item.sourceUrl, null);
    assert.equal(item.coverUrl, null);
  });

  it('removes an issue and its photo', async () => {
    const list = await (await fetch(`${base}/api/collection`)).json();
    const odd = list.items.find((item) => item.series === 'Odd');
    const removed = await fetch(`${base}/api/comics/${odd.id}`, { method: 'DELETE' });
    assert.equal(removed.status, 204);
    assert.equal((await fetch(`${base}${odd.photoUrl}`)).status, 404);
    assert.equal((await fetch(`${base}/api/comics/${odd.id}`, { method: 'DELETE' })).status, 404);
  });

  it('returns JSON errors for unknown API routes', async () => {
    const response = await fetch(`${base}/api/nope`);
    assert.equal(response.status, 404);
    assert.deepEqual(await response.json(), { error: 'Not found' });
  });
});
