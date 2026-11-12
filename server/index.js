import { createApp } from './app.js';
import { loadConfig } from './config.js';
import { openCollectionDb } from './db.js';
import { openInducksDb } from './metadata/inducks.js';

const config = loadConfig();
const db = openCollectionDb(config.collectionDbPath);
const inducksDb = openInducksDb(config.inducksDbPath);
const app = createApp({ config, db, inducksDb });

app.listen(config.port, () => {
  console.log(`Comic collection listening on http://localhost:${config.port}`);
  console.log(`Collection database: ${config.collectionDbPath}`);
  console.log(
    inducksDb
      ? `Inducks database: ${config.inducksDbPath}`
      : `Inducks database not found at ${config.inducksDbPath} (run npm run sync:inducks)`,
  );
});
