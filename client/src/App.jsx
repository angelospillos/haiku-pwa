import { useCallback, useEffect, useState } from 'preact/hooks';
import { api } from './api.js';
import { CheckIssue } from './CheckIssue.jsx';
import { CollectionList } from './CollectionList.jsx';

const SERIES_STORAGE_KEY = 'comic-collection.series';

export function App() {
  const [view, setView] = useState('check');
  const [series, setSeries] = useState(() => localStorage.getItem(SERIES_STORAGE_KEY) ?? '');
  const [items, setItems] = useState([]);
  const [seriesNames, setSeriesNames] = useState([]);
  const [loadError, setLoadError] = useState('');
  const [toast, setToast] = useState('');

  const refresh = useCallback(async () => {
    try {
      const data = await api.list();
      setItems(data.items);
      setSeriesNames(data.series);
      setLoadError('');
    } catch (error) {
      setLoadError(error.message);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    localStorage.setItem(SERIES_STORAGE_KEY, series);
  }, [series]);

  useEffect(() => {
    if (!toast) return undefined;
    const timer = setTimeout(() => setToast(''), 4000);
    return () => clearTimeout(timer);
  }, [toast]);

  async function removeItem(item) {
    if (!window.confirm(`Remove ${item.series} #${item.number} from your collection?`)) return;
    try {
      await api.remove(item.id);
      setToast(`Removed ${item.series} #${item.number}`);
      refresh();
    } catch (error) {
      setLoadError(error.message);
    }
  }

  return (
    <div class="app">
      <header class="topbar">
        <h1>Comic Collection</h1>
        <button
          type="button"
          class="secondary"
          onClick={() => setView(view === 'check' ? 'collection' : 'check')}
        >
          {view === 'check' ? `My collection (${items.length})` : 'Check an issue'}
        </button>
      </header>

      <main>
        {loadError && (
          <p class="alert" role="alert">
            {loadError}
          </p>
        )}

        {view === 'check' ? (
          <CheckIssue
            series={series}
            onSeriesChange={setSeries}
            seriesOptions={seriesNames}
            onSaved={(item) => {
              setToast(`Added ${item.series} #${item.number} to your collection`);
              refresh();
            }}
          />
        ) : (
          <CollectionList items={items} onRemove={removeItem} />
        )}
      </main>

      <div class="toast" role="status" aria-live="polite">
        {toast}
      </div>
    </div>
  );
}
