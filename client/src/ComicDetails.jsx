import { SOURCE_NAMES, formatDate } from './format.js';

export function ComicDetails({ item }) {
  const hasCatalogue = Boolean(item.source);
  return (
    <ul class="details">
      {item.title && <li>{item.title}</li>}
      {item.seriesTitle && <li class="muted">{item.seriesTitle}</li>}
      {item.publicationDate && <li class="muted">Published {item.publicationDate}</li>}
      <li class="muted">Added {formatDate(item.createdAt)}</li>
      {hasCatalogue ? (
        <li>
          <a href={item.sourceUrl} target="_blank" rel="noreferrer">
            Details on {SOURCE_NAMES[item.source] ?? item.source} ↗
          </a>
        </li>
      ) : (
        <li class="muted">No catalogue details saved</li>
      )}
    </ul>
  );
}
