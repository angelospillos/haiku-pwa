import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';

const here = path.dirname(fileURLToPath(import.meta.url));
const PHOTO = path.join(here, '../fixtures/comic-photo.jpg');
const SHOTS = path.join(here, '../../docs/screenshots');

async function shot(page, testInfo, name) {
  if (testInfo.project.name !== 'phone') return;
  await page.screenshot({ path: path.join(SHOTS, `${name}.png`), fullPage: true });
}

async function emptyCollection(request) {
  const { items } = await (await request.get('/api/collection')).json();
  for (const item of items) {
    await request.delete(`/api/comics/${item.id}`);
  }
}

async function checkIssue(page, series, number, year) {
  await page.getByLabel('Series').fill(series);
  await page.getByLabel('Issue number').fill(number);
  await page.getByLabel('Year (optional)').fill(year ?? '');
  await page.getByRole('button', { name: 'Check issue' }).click();
}

async function openCollection(page) {
  await page.getByRole('button', { name: /^My collection \(\d+\)$/ }).click();
  await expect(page.getByRole('button', { name: 'Check an issue' })).toBeVisible();
}

async function removeFromCollection(page, label) {
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: `Remove ${label}` }).click();
  await expect(page.getByRole('button', { name: `Remove ${label}` })).toHaveCount(0);
}

test.describe('comic collection', () => {
  test.beforeEach(async ({ page }) => {
    await emptyCollection(page.request);
    await page.goto('/');
  });

  test.afterEach(async ({ page }) => {
    await emptyCollection(page.request);
  });

  test('a new issue is matched, photographed, saved, found again, listed and removed', async ({ page }, testInfo) => {
    await expect(page.getByRole('heading', { name: 'Comic Collection' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'My collection (0)' })).toBeVisible();
    await shot(page, testInfo, '01-start');

    await checkIssue(page, 'Asterix', '5', '1965');
    await shot(page, testInfo, '02-issue-entered');

    await expect(page.getByRole('heading', { name: "Asterix #5 isn't in your collection yet" })).toBeVisible();
    await expect(page.getByText('comics.org: 1 match')).toBeVisible();
    await expect(page.getByText('Inducks: 0 matches')).toBeVisible();
    await expect(page.locator('img[src$="/covers/273771.png"]')).toHaveJSProperty('naturalWidth', 400);

    const match = page.getByLabel(/Le tour de Gaule/);
    await match.check();
    await expect(match).toBeChecked();
    await shot(page, testInfo, '03-matches-found');
    if (testInfo.project.name === 'desktop') {
      await page.screenshot({ path: path.join(SHOTS, 'desktop-03-matches-found.png'), fullPage: true });
    }

    await page.getByLabel('Take photo of the comic').setInputFiles(PHOTO);
    await expect(page.getByAltText('Photo of the comic to be saved')).toBeVisible();
    await shot(page, testInfo, '04-photo-ready');

    await page.getByRole('button', { name: 'Add to collection' }).click();
    await expect(page.getByText('Added Asterix #5 to your collection')).toBeVisible();
    await expect(page.getByRole('button', { name: 'My collection (1)' })).toBeVisible();
    await shot(page, testInfo, '05-added');

    await checkIssue(page, 'asterix', '05');
    await expect(page.getByText('Already in your collection')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Asterix #5' })).toBeVisible();
    await expect(page.getByAltText('Your photo of Asterix #5')).toHaveJSProperty('naturalWidth', 960);
    await shot(page, testInfo, '06-already-owned');

    await openCollection(page);
    const row = page.getByRole('listitem').filter({ hasText: 'Asterix #5' });
    await expect(row).toContainText('Le tour de Gaule');
    await expect(row.getByRole('link', { name: /Details on comics\.org/ })).toHaveAttribute(
      'href',
      'https://www.comics.org/issue/273771/',
    );
    await shot(page, testInfo, '07-my-collection');

    await removeFromCollection(page, 'Asterix #5');
    await expect(page.getByText('Nothing here yet')).toBeVisible();
  });

  test('an issue no source knows about can still be saved with a photo', async ({ page }, testInfo) => {
    await checkIssue(page, 'Asterix', '999');

    await expect(page.getByRole('heading', { name: "Asterix #999 isn't in your collection yet" })).toBeVisible();
    await expect(page.getByText('No matches found. You can still save it without details.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add to collection' })).toBeDisabled();
    await expect(page.getByText('A photo is needed before the issue can be saved.')).toBeVisible();

    await page.getByLabel('Take photo of the comic').setInputFiles(PHOTO);
    await page.getByRole('button', { name: 'Add to collection' }).click();
    await expect(page.getByText('Added Asterix #999 to your collection')).toBeVisible();

    await openCollection(page);
    const row = page.getByRole('listitem').filter({ hasText: 'Asterix #999' });
    await expect(row).toContainText('No catalogue details saved');
    await shot(page, testInfo, '08-saved-without-details');

    await removeFromCollection(page, 'Asterix #999');
  });

  test('an Inducks match is offered for Disney series and linked after saving', async ({ page }, testInfo) => {
    await checkIssue(page, 'Topolino', '3401');

    await expect(page.getByRole('heading', { name: "Topolino #3401 isn't in your collection yet" })).toBeVisible();
    await expect(page.getByText('Inducks: 1 match')).toBeVisible();
    await page.getByLabel(/Topolino \(libretto\) · Italy/).check();
    await page.getByLabel('Take photo of the comic').setInputFiles(PHOTO);
    await shot(page, testInfo, '09-inducks-match');
    await page.getByRole('button', { name: 'Add to collection' }).click();
    await expect(page.getByText('Added Topolino #3401 to your collection')).toBeVisible();

    await openCollection(page);
    const row = page.getByRole('listitem').filter({ hasText: 'Topolino #3401' });
    await expect(row.getByRole('link', { name: /Details on Inducks/ })).toHaveAttribute(
      'href',
      'https://inducks.org/issue.php?c=it%2FTL+3401',
    );

    await removeFromCollection(page, 'Topolino #3401');
  });

  test('a failing comics.org does not block the lookup or saving', async ({ page }, testInfo) => {
    await checkIssue(page, 'Broken Saga', '1');

    await expect(page.getByText('comics.org: unavailable (comics.org responded with HTTP 500)')).toBeVisible();
    await expect(page.getByText('Inducks: 0 matches')).toBeVisible();
    await page.getByLabel('Take photo of the comic').setInputFiles(PHOTO);
    await page.getByRole('button', { name: 'Add to collection' }).click();
    await expect(page.getByText('Added Broken Saga #1 to your collection')).toBeVisible();
    await shot(page, testInfo, '10-source-unavailable');
  });

  test('empty fields are explained instead of sent', async ({ page }, testInfo) => {
    await page.getByRole('button', { name: 'Check issue' }).click();
    await expect(page.getByRole('alert')).toHaveText('Enter the series first, for example Asterix.');

    await page.getByLabel('Series').fill('Asterix');
    await page.getByRole('button', { name: 'Check issue' }).click();
    await expect(page.getByRole('alert')).toHaveText('Enter an issue number.');
    await shot(page, testInfo, '11-validation');
  });

  test('the app is an installable PWA that also opens offline', async ({ page, context }) => {
    const manifest = await page.request.get('/manifest.webmanifest');
    expect(manifest.ok()).toBeTruthy();
    const json = await manifest.json();
    expect(json).toMatchObject({ name: 'Comic Collection', display: 'standalone' });
    expect(json.icons.map((icon) => icon.sizes)).toEqual(expect.arrayContaining(['192x192', '512x512']));

    await page.evaluate(() => navigator.serviceWorker.ready.then(() => true));
    await page.reload();
    await expect
      .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)), { timeout: 15_000 })
      .toBe(true);

    await context.setOffline(true);
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Comic Collection' })).toBeVisible();
    await context.setOffline(false);
  });
});
