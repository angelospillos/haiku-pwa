export const SOURCE_NAMES = {
  'comics.org': 'comics.org',
  inducks: 'Inducks',
};

export function formatDate(iso) {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
