import { CACHE_TTL_MS, USER_AGENT, cleanText, mapLimit } from './util.js';

const MAX_CANDIDATES = 8;
const DETAIL_CONCURRENCY = 3;
const TIMEOUT_MS = 10_000;

export class SourceError extends Error {}

export function createComicsOrgClient({ baseUrl, cache, fetchImpl = fetch }) {
  async function getJson(url) {
    const cached = cache.get(`comicsorg:${url}`, CACHE_TTL_MS);
    if (cached) return cached;

    let response;
    try {
      response = await fetchImpl(url, {
        headers: { Accept: 'application/json', 'User-Agent': USER_AGENT },
        signal: AbortSignal.timeout(TIMEOUT_MS),
      });
    } catch (error) {
      throw new SourceError(`comics.org could not be reached (${error.name})`);
    }
    if (response.status === 404) return null;
    if (!response.ok) throw new SourceError(`comics.org responded with HTTP ${response.status}`);

    const json = await response.json();
    cache.set(`comicsorg:${url}`, json);
    return json;
  }

  function issueIdFrom(apiUrl) {
    const match = /\/issue\/(\d+)\//.exec(apiUrl ?? '');
    return match ? match[1] : null;
  }

  function toCandidate(summary, detail) {
    const sourceId = issueIdFrom(summary.api_url);
    return {
      source: 'comics.org',
      sourceId,
      sourceUrl: sourceId ? `https://www.comics.org/issue/${sourceId}/` : null,
      seriesTitle: cleanText(summary.series_name, 200),
      title: cleanText(detail?.title || summary.descriptor, 200),
      publicationDate: cleanText(detail?.publication_date || summary.publication_date, 40),
      coverUrl: /^https?:\/\//.test(detail?.cover ?? '') ? detail.cover : null,
    };
  }

  return {
    async searchIssues({ series, number, year }) {
      const base = `${baseUrl}/series/name/${encodeURIComponent(series)}/issue/${encodeURIComponent(number)}/`;
      const url = `${base}${year ? `year/${encodeURIComponent(year)}/` : ''}?format=json`;
      const list = await getJson(url);
      const summaries = (list?.results ?? []).filter((item) => !item.variant_of).slice(0, MAX_CANDIDATES);

      const details = await mapLimit(summaries, DETAIL_CONCURRENCY, async (summary) => {
        if (!summary.api_url) return null;
        try {
          return await getJson(summary.api_url);
        } catch {
          return null;
        }
      });

      return summaries
        .map((summary, index) => toCandidate(summary, details[index]))
        .filter((candidate) => candidate.sourceId);
    },
  };
}
