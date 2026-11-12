// Generates the image fixtures used by the tests:
//   cover.png      – the "cover" that the stub comics.org API serves
//   comic-photo.jpg – a larger, photo-like picture a user would upload
// Run with `node tests/fixtures/make-fixture-images.mjs`.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';

const here = path.dirname(fileURLToPath(import.meta.url));

const drawCover = () => {
  const canvas = document.createElement('canvas');
  canvas.width = 400;
  canvas.height = 600;
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#1d4ed8';
  ctx.fillRect(0, 0, 400, 600);
  ctx.fillStyle = '#f59e0b';
  ctx.fillRect(0, 0, 400, 150);
  ctx.fillStyle = '#ffffff';
  ctx.font = 'bold 64px Arial';
  ctx.textAlign = 'center';
  ctx.fillText('ASTÉRIX', 200, 98);
  ctx.beginPath();
  ctx.arc(200, 330, 120, 0, Math.PI * 2);
  ctx.fillStyle = '#fde68a';
  ctx.fill();
  ctx.fillStyle = '#1d4ed8';
  ctx.font = 'bold 120px Arial';
  ctx.fillText('5', 200, 380);
  ctx.fillStyle = '#ffffff';
  ctx.font = 'bold 30px Arial';
  ctx.fillText('LE TOUR DE GAULE', 200, 520);
  return canvas.toDataURL('image/png');
};

const drawPhoto = () => {
  const canvas = document.createElement('canvas');
  canvas.width = 1200;
  canvas.height = 1600;
  const ctx = canvas.getContext('2d');
  const wood = ctx.createLinearGradient(0, 0, 1200, 1600);
  wood.addColorStop(0, '#8b5a2b');
  wood.addColorStop(1, '#4a2c12');
  ctx.fillStyle = wood;
  ctx.fillRect(0, 0, 1200, 1600);
  ctx.save();
  ctx.translate(600, 800);
  ctx.rotate(-0.06);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(-380, -520, 760, 1040);
  ctx.fillStyle = '#dc2626';
  ctx.fillRect(-350, -490, 700, 980);
  ctx.fillStyle = '#fde047';
  ctx.beginPath();
  ctx.arc(0, -60, 230, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = '#111827';
  ctx.font = 'bold 200px Arial';
  ctx.textAlign = 'center';
  ctx.fillText('5', 0, 150);
  ctx.font = 'bold 70px Arial';
  ctx.fillText('ASTÉRIX', 0, -330);
  ctx.restore();
  return canvas.toDataURL('image/jpeg', 0.92);
};

const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  await page.setContent('<html><body></body></html>');
  const cover = await page.evaluate(`(${drawCover.toString()})()`);
  const photo = await page.evaluate(`(${drawPhoto.toString()})()`);
  fs.writeFileSync(path.join(here, 'cover.png'), Buffer.from(cover.split(',')[1], 'base64'));
  fs.writeFileSync(path.join(here, 'comic-photo.jpg'), Buffer.from(photo.split(',')[1], 'base64'));
  console.log('wrote tests/fixtures/cover.png and tests/fixtures/comic-photo.jpg');
} finally {
  await browser.close();
}
