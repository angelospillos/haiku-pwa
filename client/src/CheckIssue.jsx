import { useRef, useState } from 'preact/hooks';
import { api } from './api.js';
import { ComicDetails } from './ComicDetails.jsx';
import { fileToJpegDataUrl } from './photo.js';
import { SOURCE_NAMES } from './format.js';

const IDLE_LOOKUP = { loading: false, data: null, error: '' };

function MatchList({ lookup, choice, onChoice }) {
  if (lookup.loading) {
    return <p class="muted" role="status">Searching comics.org and Inducks…</p>;
  }
  if (lookup.error) {
    return <p class="alert" role="alert">Lookup failed: {lookup.error}</p>;
  }
  if (!lookup.data) return null;

  const { candidates, sources } = lookup.data;
  return (
    <>
      <ul class="sources">
        {Object.entries(sources).map(([name, status]) => (
          <li key={name}>
            <strong>{SOURCE_NAMES[name] ?? name}:</strong>{' '}
            {status.ok ? `${status.count} match${status.count === 1 ? '' : 'es'}` : `unavailable (${status.message})`}
          </li>
        ))}
      </ul>

      {candidates.length === 0 && (
        <p class="muted">No matches found. You can still save it without details.</p>
      )}

      <fieldset class="candidates">
        <legend>Which issue is it?</legend>
        {candidates.map((candidate, index) => (
          <label class="candidate" key={`${candidate.source}-${candidate.sourceId}`}>
            <input
              type="radio"
              name="match"
              value={String(index)}
              checked={choice === String(index)}
              onChange={() => onChoice(String(index))}
            />
            {candidate.coverUrl ? (
              <img src={candidate.coverUrl} alt="" loading="lazy" />
            ) : (
              <span class="cover-placeholder" aria-hidden="true">
                {SOURCE_NAMES[candidate.source]}
              </span>
            )}
            <span class="candidate-text">
              <strong>{candidate.seriesTitle}</strong>
              {candidate.title && <span>{candidate.title}</span>}
              {candidate.publicationDate && <span class="muted">{candidate.publicationDate}</span>}
              <a href={candidate.sourceUrl} target="_blank" rel="noreferrer">
                View on {SOURCE_NAMES[candidate.source]} ↗
              </a>
            </span>
          </label>
        ))}
        <label class="candidate none">
          <input
            type="radio"
            name="match"
            value="none"
            checked={choice === 'none'}
            onChange={() => onChoice('none')}
          />
          <span>{candidates.length ? 'None of these: save without details' : 'Save without details'}</span>
        </label>
      </fieldset>
    </>
  );
}

function OwnedCard({ item, onAgain }) {
  return (
    <section class="card owned" aria-labelledby="owned-title">
      <p class="badge">✓ Already in your collection</p>
      <div class="owned-body">
        <img src={item.photoUrl} alt={`Your photo of ${item.series} #${item.number}`} />
        <div>
          <h2 id="owned-title">
            {item.series} #{item.number}
          </h2>
          <ComicDetails item={item} />
        </div>
      </div>
      <button type="button" class="secondary" onClick={onAgain}>
        Check another issue
      </button>
    </section>
  );
}

function MissingIssue({ checked, lookup, choice, onChoice, photo, onPhoto, onSave, onCancel, saving }) {
  return (
    <section class="card" aria-labelledby="missing-title">
      <h2 id="missing-title">
        {checked.series} #{checked.number} isn't in your collection yet
      </h2>

      <h3>1. Pick the matching issue (optional)</h3>
      <MatchList lookup={lookup} choice={choice} onChoice={onChoice} />

      <h3>2. Photograph the cover</h3>
      <label class="button secondary file-button">
        📷 {photo ? 'Retake photo' : 'Take photo'}
        <input
          type="file"
          accept="image/*"
          capture="environment"
          class="sr-only"
          aria-label="Take photo of the comic"
          onChange={onPhoto}
        />
      </label>
      {photo && <img class="preview" src={photo.dataUrl} alt="Photo of the comic to be saved" />}

      <div class="actions">
        <button type="button" class="primary" disabled={!photo || saving} onClick={onSave}>
          {saving ? 'Saving…' : 'Add to collection'}
        </button>
        <button type="button" class="ghost" onClick={onCancel}>
          Cancel
        </button>
      </div>
      {!photo && <p class="muted hint">A photo is needed before the issue can be saved.</p>}
    </section>
  );
}

export function CheckIssue({ series, onSeriesChange, seriesOptions, onSaved }) {
  const [number, setNumber] = useState('');
  const [year, setYear] = useState('');
  const [phase, setPhase] = useState('idle');
  const [owned, setOwned] = useState(null);
  const [checked, setChecked] = useState(null);
  const [lookup, setLookup] = useState(IDLE_LOOKUP);
  const [choice, setChoice] = useState('none');
  const [photo, setPhoto] = useState(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const requestRef = useRef(0);

  function reset() {
    requestRef.current++;
    setNumber('');
    setPhase('idle');
    setOwned(null);
    setChecked(null);
    setLookup(IDLE_LOOKUP);
    setChoice('none');
    setPhoto(null);
    setError('');
  }

  async function runLookup(requestId, seriesName, issueNumber, issueYear) {
    try {
      const data = await api.lookup(seriesName, issueNumber, issueYear);
      if (requestId === requestRef.current) setLookup({ loading: false, data, error: '' });
    } catch (err) {
      if (requestId === requestRef.current) setLookup({ loading: false, data: null, error: err.message });
    }
  }

  async function onCheck(event) {
    event.preventDefault();
    const seriesName = series.trim();
    const issueNumber = number.trim();
    if (!seriesName) return setError('Enter the series first, for example Asterix.');
    if (!issueNumber) return setError('Enter an issue number.');

    const requestId = ++requestRef.current;
    setError('');
    setPhase('checking');
    setOwned(null);
    setPhoto(null);
    setChoice('none');
    try {
      const result = await api.check(seriesName, issueNumber);
      if (requestId !== requestRef.current) return;
      if (result.owned) {
        setOwned(result.item);
        setPhase('owned');
        return;
      }
      const issueYear = year.trim();
      setChecked({ series: seriesName, number: issueNumber, year: issueYear });
      setLookup({ loading: true, data: null, error: '' });
      setPhase('missing');
      runLookup(requestId, seriesName, issueNumber, issueYear);
    } catch (err) {
      if (requestId !== requestRef.current) return;
      setError(err.message);
      setPhase('idle');
    }
  }

  async function onPhoto(event) {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = '';
    if (!file) return;
    try {
      setPhoto({ dataUrl: await fileToJpegDataUrl(file) });
      setError('');
    } catch {
      setError("That picture couldn't be read. Try taking it again.");
    }
  }

  async function onSave() {
    const candidate = choice === 'none' ? null : lookup.data?.candidates[Number(choice)];
    setSaving(true);
    setError('');
    try {
      const { item } = await api.add({
        series: checked.series,
        number: checked.number,
        photo: photo.dataUrl,
        match: candidate ?? undefined,
      });
      onSaved(item);
      reset();
    } catch (err) {
      if (err.status === 409 && err.item) {
        setOwned(err.item);
        setPhase('owned');
      } else {
        setError(err.message);
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div class="check">
      <form class="check-form card" onSubmit={onCheck}>
        <label class="field">
          <span>Series</span>
          <input
            type="text"
            list="series-options"
            value={series}
            placeholder="e.g. Asterix"
            autocomplete="off"
            onInput={(event) => onSeriesChange(event.currentTarget.value)}
          />
        </label>
        <datalist id="series-options">
          {seriesOptions.map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>

        <div class="row">
          <label class="field grow">
            <span>Issue number</span>
            <input
              type="text"
              inputmode="text"
              value={number}
              placeholder="e.g. 5"
              autocomplete="off"
              onInput={(event) => setNumber(event.currentTarget.value)}
            />
          </label>
          <label class="field year">
            <span>Year (optional)</span>
            <input
              type="text"
              inputmode="numeric"
              maxLength={4}
              value={year}
              placeholder="1965"
              autocomplete="off"
              onInput={(event) => setYear(event.currentTarget.value)}
            />
          </label>
        </div>

        <button type="submit" class="primary" disabled={phase === 'checking'}>
          {phase === 'checking' ? 'Checking…' : 'Check issue'}
        </button>
      </form>

      {error && (
        <p class="alert" role="alert">
          {error}
        </p>
      )}

      {phase === 'owned' && owned && <OwnedCard item={owned} onAgain={reset} />}
      {phase === 'missing' && checked && (
        <MissingIssue
          checked={checked}
          lookup={lookup}
          choice={choice}
          onChoice={setChoice}
          photo={photo}
          onPhoto={onPhoto}
          onSave={onSave}
          onCancel={reset}
          saving={saving}
        />
      )}
    </div>
  );
}
