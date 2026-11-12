import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig, devices } from '@playwright/test';

const root = path.dirname(fileURLToPath(import.meta.url));
const APP_PORT = 4173;
const STUB_PORT = 4010;
const testData = path.join(root, 'tests/.tmp');

export default defineConfig({
  testDir: 'tests/e2e',
  testMatch: '**/*.spec.js',
  timeout: 60_000,
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: `http://localhost:${APP_PORT}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'phone', use: { ...devices['Pixel 7'] } },
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 900 } } },
  ],
  webServer: [
    {
      command: 'node tests/fixtures/stub-comics-org.js',
      url: `http://localhost:${STUB_PORT}/health`,
      reuseExistingServer: false,
      env: { STUB_PORT: String(STUB_PORT) },
    },
    {
      command: 'npm run build --silent && make --no-print-directory build && sh tests/e2e/start-server.sh',
      url: `http://localhost:${APP_PORT}/api/health`,
      reuseExistingServer: false,
      env: {
        PORT: String(APP_PORT),
        DATA_DIR: path.join(testData, 'data'),
        INDUCKS_DB_PATH: path.join(testData, 'inducks.sqlite'),
        COMICS_ORG_API_URL: `http://localhost:${STUB_PORT}/api`,
      },
    },
  ],
});
