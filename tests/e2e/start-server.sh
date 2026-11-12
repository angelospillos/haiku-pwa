#!/bin/sh
# Starts the real server for the browser tests: a clean data directory, the
# small Inducks fixture imported into its own index, and the stub comics.org API
# configured through COMICS_ORG_API_URL (see playwright.config.js).
set -eu

: "${DATA_DIR:?DATA_DIR must be set}"
: "${INDUCKS_DB_PATH:?INDUCKS_DB_PATH must be set}"

rm -rf "$DATA_DIR"
rm -f "$INDUCKS_DB_PATH"
./bin/sync-inducks -dir tests/fixtures/inducks/isv
exec ./bin/server
