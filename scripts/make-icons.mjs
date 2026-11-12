// Renders the app icons (SVG + PNG) from one design using the Chromium that
// Playwright already installs for the end-to-end tests. Run with `npm run icons`.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const outDir = path.join(root, 'client/public/icons');
const BLUE = '#1d4ed8';
const GOLD = '#f59e0b';

function iconSvg({ maskable = false } = {}) {
  const scale = maskable ? 0.72 : 1;
  const background = maskable
    ? `<rect width="512" height="512" fill="${BLUE}"/>`
    : `<rect width="512" height="512" rx="112" fill="${BLUE}"/>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  ${background}
  <g transform="translate(256 256) scale(${scale}) translate(-256 -256)">
    <rect x="124" y="96" width="264" height="330" rx="26" fill="#ffffff"/>
    <rect x="124" y="96" width="264" height="84" rx="26" fill="${GOLD}"/>
    <rect x="124" y="140" width="264" height="40" fill="${GOLD}"/>
    <text x="256" y="372" text-anchor="middle" font-family="Arial Black, Arial, sans-serif" font-weight="900" font-size="150" fill="${BLUE}">#1</text>
  </g>
</svg>
`;
}

const targets = [
  { file: 'icon-192.png', size: 192, svg: iconSvg() },
  { file: 'icon-512.png', size: 512, svg: iconSvg() },
  { file: 'icon-512-maskable.png', size: 512, svg: iconSvg({ maskable: true }) },
  { file: 'apple-touch-icon.png', size: 180, svg: iconSvg({ maskable: true }) },
];

fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(path.join(outDir, 'icon.svg'), iconSvg());

const browser = await chromium.launch();
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  for (const { file, size, svg } of targets) {
    await page.setViewportSize({ width: size, height: size });
    const sized = svg.replace('<svg ', `<svg width="${size}" height="${size}" `);
    await page.setContent(`<html><body style="margin:0;background:transparent">${sized}</body></html>`);
    await page.screenshot({ path: path.join(outDir, file), omitBackground: true, clip: { x: 0, y: 0, width: size, height: size } });
    console.log(`wrote ${path.join('client/public/icons', file)}`);
  }
} finally {
  await browser.close();
}
