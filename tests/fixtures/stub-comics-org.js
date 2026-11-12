// A stand-in for the comics.org API used by the end-to-end tests, so they never
// depend on the live site. It mirrors the response shapes documented at
// https://www.comics.org/api/schema/redoc. Series named "Broken …" return HTTP 500.

import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const PORT = Number(process.env.STUB_PORT || 4010);
const BASE = `http://localhost:${PORT}`;
const cover = fs.readFileSync(path.join(here, 'cover.png'));

const ISSUES = [
  { id: 273771, series: 'Astérix (1961 series)', number: '5', title: 'Le tour de Gaule', date: '1965', variantOf: null },
  { id: 2404546, series: 'Asterix (1968 series)', number: '5', title: 'Die goldene Sichel', date: '1971', variantOf: null },
  { id: 2404547, series: 'Asterix (1968 series)', number: '5', title: 'Die goldene Sichel (variant)', date: '1971', variantOf: 2404546 },
];

const normalize = (value) =>
  value.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase().trim();

function summary(issue) {
  return {
    api_url: `${BASE}/api/issue/${issue.id}/?format=json`,
    series_name: issue.series,
    descriptor: `${issue.number} - ${issue.title}`,
    publication_date: issue.date,
    price: '',
    page_count: null,
    variant_of: issue.variantOf ? `${BASE}/api/issue/${issue.variantOf}/?format=json` : null,
    series: `${BASE}/api/series/1/?format=json`,
  };
}

function detail(issue) {
  return {
    api_url: `${BASE}/api/issue/${issue.id}/?format=json`,
    series_name: issue.series,
    descriptor: `${issue.number} - ${issue.title}`,
    number: issue.number,
    title: issue.title,
    publication_date: issue.date,
    indicia_publisher: null,
    cover: `${BASE}/covers/${issue.id}.png`,
  };
}

function send(res, status, body, type = 'application/json') {
  res.writeHead(status, { 'Content-Type': type });
  res.end(type === 'application/json' ? JSON.stringify(body) : body);
}

const server = http.createServer((req, res) => {
  const url = new URL(req.url, BASE);
  const seriesMatch = /^\/api\/series\/name\/([^/]+)\/issue\/([^/]+)\/(?:year\/(\d{4})\/)?$/.exec(url.pathname);

  if (url.pathname === '/health') return send(res, 200, { ok: true });

  if (url.pathname.startsWith('/covers/')) return send(res, 200, cover, 'image/png');

  if (seriesMatch) {
    const [, rawName, rawNumber, year] = seriesMatch;
    const name = normalize(decodeURIComponent(rawName));
    const number = decodeURIComponent(rawNumber);
    if (name.startsWith('broken')) return send(res, 500, { detail: 'stub failure' });

    const results = ISSUES.filter(
      (issue) =>
        normalize(issue.series).startsWith(name) &&
        issue.number === number &&
        (!year || issue.date === year),
    ).map(summary);
    return send(res, 200, { count: results.length, next: null, previous: null, results });
  }

  const issueMatch = /^\/api\/issue\/(\d+)\/$/.exec(url.pathname);
  if (issueMatch) {
    const issue = ISSUES.find((candidate) => candidate.id === Number(issueMatch[1]));
    return issue ? send(res, 200, detail(issue)) : send(res, 404, { detail: 'Not found.' });
  }

  return send(res, 404, { detail: 'Not found.' });
});

server.listen(PORT, () => {
  console.log(`Stub comics.org API on ${BASE}`);
});
