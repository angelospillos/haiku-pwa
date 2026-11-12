import { ComicDetails } from './ComicDetails.jsx';

export function CollectionList({ items, onRemove }) {
  if (items.length === 0) {
    return (
      <section class="card">
        <h2>My collection</h2>
        <p class="empty">Nothing here yet. Check an issue to add your first one.</p>
      </section>
    );
  }

  return (
    <section>
      <h2>
        My collection <span class="muted">({items.length})</span>
      </h2>
      <ul class="collection">
        {items.map((item) => (
          <li key={item.id} class="collection-item">
            <img src={item.photoUrl} alt={`Photo of ${item.series} #${item.number}`} loading="lazy" />
            <div class="collection-text">
              <h3>
                {item.series} #{item.number}
              </h3>
              <ComicDetails item={item} />
            </div>
            <button
              type="button"
              class="ghost danger"
              aria-label={`Remove ${item.series} #${item.number}`}
              onClick={() => onRemove(item)}
            >
              Remove
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
