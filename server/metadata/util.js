export const USER_AGENT = 'haiku-pwa/0.1 (personal comic collection; +https://github.com/angelospillos/haiku-pwa)';

export const CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

export async function mapLimit(items, limit, fn) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) {
      const index = next++;
      results[index] = await fn(items[index], index);
    }
  });
  await Promise.all(workers);
  return results;
}

export function cleanText(value, max) {
  if (typeof value !== 'string') return '';
  return value.replace(/\s+/g, ' ').trim().slice(0, max);
}
