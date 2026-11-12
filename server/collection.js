import fs from 'node:fs';
import path from 'node:path';

export class DuplicateComicError extends Error {
  constructor(existing) {
    super('This issue is already in the collection');
    this.name = 'DuplicateComicError';
    this.existing = existing;
  }
}

export function seriesKey(series) {
  return series
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim();
}

export function numberKey(number) {
  return number.trim().toUpperCase().replace(/\s+/g, '').replace(/^0+(?=\d)/, '');
}

function toItem(row) {
  return {
    id: row.id,
    series: row.series,
    number: row.number,
    source: row.source,
    sourceUrl: row.source_url,
    seriesTitle: row.series_title,
    title: row.title,
    publicationDate: row.publication_date,
    coverUrl: row.cover_url,
    photoUrl: `/api/comics/${row.id}/photo`,
    createdAt: row.created_at,
  };
}

function inTransaction(db, fn) {
  db.exec('BEGIN IMMEDIATE');
  try {
    const result = fn();
    db.exec('COMMIT');
    return result;
  } catch (error) {
    db.exec('ROLLBACK');
    throw error;
  }
}

export function createCollection(db, photosDir) {
  const selectAll = db.prepare(
    'SELECT * FROM comics ORDER BY series_key, CAST(number_key AS INTEGER), number_key',
  );
  const selectByKey = db.prepare('SELECT * FROM comics WHERE series_key = ? AND number_key = ?');
  const selectById = db.prepare('SELECT * FROM comics WHERE id = ?');
  const selectSeries = db.prepare('SELECT DISTINCT series FROM comics ORDER BY series');
  const insert = db.prepare(`
    INSERT INTO comics (
      series, series_key, number, number_key, source, source_id, source_url,
      series_title, title, publication_date, cover_url, photo_file, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);
  const remove = db.prepare('DELETE FROM comics WHERE id = ?');

  const collection = {
    list() {
      return selectAll.all().map(toItem);
    },

    seriesNames() {
      return selectSeries.all().map((row) => row.series);
    },

    find(series, number) {
      const row = selectByKey.get(seriesKey(series), numberKey(number));
      return row ? toItem(row) : null;
    },

    get(id) {
      const row = selectById.get(id);
      return row ? toItem(row) : null;
    },

    photoPath(id) {
      const row = selectById.get(id);
      return row ? path.join(photosDir, row.photo_file) : null;
    },

    add({ series, number, photo, match }) {
      const photoFile = `${crypto.randomUUID()}.${photo.extension}`;
      const createdAt = new Date().toISOString();
      const existing = collection.find(series, number);
      if (existing) throw new DuplicateComicError(existing);

      return inTransaction(db, () => {
        fs.mkdirSync(photosDir, { recursive: true });
        try {
          const result = insert.run(
            series,
            seriesKey(series),
            number,
            numberKey(number),
            match?.source ?? null,
            match?.sourceId ?? null,
            match?.sourceUrl ?? null,
            match?.seriesTitle ?? null,
            match?.title ?? null,
            match?.publicationDate ?? null,
            match?.coverUrl ?? null,
            photoFile,
            createdAt,
          );
          fs.writeFileSync(path.join(photosDir, photoFile), photo.bytes);
          return collection.get(Number(result.lastInsertRowid));
        } catch (error) {
          if (String(error.message).includes('UNIQUE constraint failed')) {
            throw new DuplicateComicError(collection.find(series, number));
          }
          throw error;
        }
      });
    },

    remove(id) {
      const row = selectById.get(id);
      if (!row) return false;
      inTransaction(db, () => {
        remove.run(id);
        fs.rmSync(path.join(photosDir, row.photo_file), { force: true });
      });
      return true;
    },
  };

  return collection;
}
