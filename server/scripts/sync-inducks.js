#!/usr/bin/env node
// Downloads the daily Inducks dump (https://inducks.org/inducks/isv.tgz) and
// imports the tables the app searches into data/inducks.sqlite.
//
// Options:
//   --archive <file.tgz>  use an already downloaded archive instead of downloading
//   --dir <isv-dir>       use already extracted .isv files instead

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { loadConfig } from '../config.js';
import { importInducks, INDUCKS_MEMBERS } from '../metadata/inducks-import.js';
import { USER_AGENT } from '../metadata/util.js';

const ARCHIVE_URL = 'https://inducks.org/inducks/isv.tgz';

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--archive') args.archive = argv[++i];
    else if (argv[i] === '--dir') args.dir = argv[++i];
    else throw new Error(`Unknown option: ${argv[i]}`);
  }
  return args;
}

async function download(url, file) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  console.log(`Downloading ${url}`);
  const response = await fetch(url, { headers: { 'User-Agent': USER_AGENT } });
  if (!response.ok) throw new Error(`Download failed with HTTP ${response.status}`);
  await pipeline(Readable.fromWeb(response.body), fs.createWriteStream(file));
  console.log(`Saved ${(fs.statSync(file).size / 1048576).toFixed(1)} MB to ${file}`);
}

function extract(archive, destDir) {
  fs.mkdirSync(destDir, { recursive: true });
  const members = INDUCKS_MEMBERS.map((name) => `isv/${name}`);
  const result = spawnSync('tar', ['-xzf', archive, '-C', destDir, ...members], { stdio: 'inherit' });
  if (result.status !== 0) throw new Error(`Could not extract ${archive}`);
}

async function main() {
  const config = loadConfig();
  const args = parseArgs(process.argv.slice(2));
  const workDir = path.join(config.dataDir, 'inducks-download');

  let isvDir = args.dir;
  if (!isvDir) {
    const archive = args.archive ?? path.join(workDir, 'isv.tgz');
    if (!args.archive) await download(ARCHIVE_URL, archive);
    console.log('Extracting the tables the app needs');
    extract(archive, workDir);
    isvDir = path.join(workDir, 'isv');
  }

  console.log(`Importing Inducks data from ${isvDir}`);
  const counts = await importInducks({ dir: isvDir, dbPath: config.inducksDbPath });
  console.log(
    `Done: ${counts.issues} issues, ${counts.publications} publications, ` +
      `${counts.publicationNames} alternative names, ${counts.countries} countries → ${config.inducksDbPath}`,
  );
  console.log('Restart the server so it picks up the new database.');
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
