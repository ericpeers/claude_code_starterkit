// SPDX-License-Identifier: MIT
// Renders an HTML brief to a Letter PDF: node render.mjs <abs.html> <abs.pdf>
import { createRequire } from 'module';

const [html, pdf] = process.argv.slice(2);
if (!html || !pdf) {
  console.error('usage: node render.mjs <abs.html> <abs.pdf>');
  process.exit(2);
}
// Resolve Playwright from the caller's project, so the skill ships no dependencies.
const require = createRequire(process.cwd() + '/');
const { chromium } = require('@playwright/test');

const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto('file://' + html);
await page.pdf({ path: pdf, format: 'Letter', preferCSSPageSize: true, printBackground: true });
await browser.close();
