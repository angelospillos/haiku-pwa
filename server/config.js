import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

export function loadConfig(env = process.env) {
  const dataDir = path.resolve(env.DATA_DIR || path.join(root, 'data'));
  return {
    port: Number(env.PORT) || 3000,
    distDir: path.join(root, 'dist'),
    dataDir,
    collectionDbPath: path.join(dataDir, 'collection.sqlite'),
    photosDir: path.join(dataDir, 'photos'),
    inducksDbPath: path.resolve(env.INDUCKS_DB_PATH || path.join(dataDir, 'inducks.sqlite')),
    comicsOrgApiUrl: (env.COMICS_ORG_API_URL || 'https://www.comics.org/api').replace(/\/$/, ''),
  };
}
