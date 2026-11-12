import fs from 'node:fs';
import path from 'node:path';
import express from 'express';
import { createHttpCache } from './cache.js';
import { createCollection, DuplicateComicError } from './collection.js';
import { lookupIssue } from './lookup.js';
import { createComicsOrgClient } from './metadata/comicsorg.js';
import { createInducksSource } from './metadata/inducks.js';
import { cleanText } from './metadata/util.js';

const MAX_PHOTO_BYTES = 10 * 1024 * 1024;
const PHOTO_EXTENSIONS = { 'image/jpeg': 'jpg', 'image/png': 'png', 'image/webp': 'webp' };
const MATCH_SOURCES = new Set(['comics.org', 'inducks']);

function parseIssueQuery(query, { allowYear = false } = {}) {
  const series = cleanText(query.series, 100);
  const number = cleanText(query.number, 20);
  const year = allowYear ? cleanText(query.year, 4) : '';
  if (!series) return { error: 'Series is required' };
  if (!number) return { error: 'Issue number is required' };
  if (year && !/^\d{4}$/.test(year)) return { error: 'Year must have four digits' };
  return { series, number, year };
}

function parsePhoto(value) {
  const match = typeof value === 'string' ? /^data:(image\/(?:jpeg|png|webp));base64,([A-Za-z0-9+/=\s]+)$/.exec(value) : null;
  if (!match) return null;
  const bytes = Buffer.from(match[2], 'base64');
  if (bytes.length === 0 || bytes.length > MAX_PHOTO_BYTES) return null;
  return { bytes, extension: PHOTO_EXTENSIONS[match[1]] };
}

function httpUrlOnly(value, max) {
  const text = cleanText(value, max);
  try {
    const url = new URL(text);
    return url.protocol === 'https:' || url.protocol === 'http:' ? text : null;
  } catch {
    return null;
  }
}

function sanitizeMatch(input) {
  if (!input || typeof input !== 'object' || !MATCH_SOURCES.has(input.source)) return null;
  return {
    source: input.source,
    sourceId: cleanText(input.sourceId, 80) || null,
    sourceUrl: httpUrlOnly(input.sourceUrl, 300),
    seriesTitle: cleanText(input.seriesTitle, 200) || null,
    title: cleanText(input.title, 200) || null,
    publicationDate: cleanText(input.publicationDate, 40) || null,
    coverUrl: httpUrlOnly(input.coverUrl, 500),
  };
}

function badRequest(res, error) {
  return res.status(400).json({ error });
}

export function createApp({ config, db, inducksDb, fetchImpl }) {
  const collection = createCollection(db, config.photosDir);
  const cache = createHttpCache(db);
  const comicsOrg = createComicsOrgClient({ baseUrl: config.comicsOrgApiUrl, cache, fetchImpl });
  const inducks = createInducksSource(inducksDb);

  const app = express();
  app.disable('x-powered-by');
  app.use(express.json({ limit: '15mb' }));

  app.get('/api/health', (req, res) => {
    res.json({ ok: true, inducks: inducks.available });
  });

  app.get('/api/collection', (req, res) => {
    res.json({ items: collection.list(), series: collection.seriesNames() });
  });

  app.get('/api/collection/check', (req, res) => {
    const query = parseIssueQuery(req.query);
    if (query.error) return badRequest(res, query.error);
    const item = collection.find(query.series, query.number);
    res.json({ owned: Boolean(item), item });
  });

  app.post('/api/collection', (req, res) => {
    const series = cleanText(req.body?.series, 100);
    const number = cleanText(req.body?.number, 20);
    if (!series) return badRequest(res, 'Series is required');
    if (!number) return badRequest(res, 'Issue number is required');

    const photo = parsePhoto(req.body?.photo);
    if (!photo) return badRequest(res, 'A photo of the comic is required (JPEG, PNG or WebP, up to 10 MB)');

    try {
      const item = collection.add({ series, number, photo, match: sanitizeMatch(req.body?.match) });
      res.status(201).json({ item });
    } catch (error) {
      if (error instanceof DuplicateComicError) {
        return res.status(409).json({ error: error.message, item: error.existing });
      }
      throw error;
    }
  });

  app.delete('/api/comics/:id', (req, res) => {
    const id = Number(req.params.id);
    if (!Number.isInteger(id)) return badRequest(res, 'Invalid id');
    if (!collection.remove(id)) return res.status(404).json({ error: 'Not found' });
    res.status(204).end();
  });

  app.get('/api/comics/:id/photo', (req, res) => {
    const id = Number(req.params.id);
    const file = Number.isInteger(id) ? collection.photoPath(id) : null;
    if (!file || !fs.existsSync(file)) return res.status(404).end();
    res.set('Cache-Control', 'private, max-age=86400');
    res.sendFile(file, { dotfiles: 'allow' });
  });

  app.get('/api/lookup', async (req, res) => {
    const query = parseIssueQuery(req.query, { allowYear: true });
    if (query.error) return badRequest(res, query.error);
    res.json(await lookupIssue({ ...query, comicsOrg, inducks }));
  });

  app.use('/api', (req, res) => {
    res.status(404).json({ error: 'Not found' });
  });

  if (fs.existsSync(config.distDir)) {
    app.use(express.static(config.distDir));
  }
  app.use((req, res, next) => {
    if (req.method !== 'GET' && req.method !== 'HEAD') return next();
    const indexFile = path.join(config.distDir, 'index.html');
    if (!fs.existsSync(indexFile)) {
      return res.status(503).type('text').send('Client not built yet. Run `npm run build` or use `npm run dev`.');
    }
    return res.sendFile(indexFile);
  });

  app.use((error, req, res, next) => {
    if (error.type === 'entity.too.large') return res.status(413).json({ error: 'Photo is too large' });
    if (error.type === 'entity.parse.failed') return res.status(400).json({ error: 'Invalid JSON' });
    console.error(error);
    return res.status(500).json({ error: 'Something went wrong on the server' });
  });

  return app;
}
