// SPDX-License-Identifier: MIT
//
// License / copyright header gate.
//
// Two kinds of files coexist in a project built from this kit:
//   * Kit-supplied boilerplate — declares `SPDX-License-Identifier: MIT` and is
//     EXEMPT here. It carries no personal copyright holder by design.
//   * Your own source — must carry a current-year copyright header naming the
//     project's configured holder (see `.copyright-holder`, written by
//     setup_dev.sh). This is how you enforce your ownership on the code you write.
//
// A fresh scaffold is green (everything shipped is SPDX-exempt). The gate starts
// enforcing once you add your own files without an SPDX tag or copyright header.

import * as fs from 'fs';
import * as path from 'path';
import { spawnSync } from 'child_process';

const ROOT = path.resolve(__dirname, '..');
const CURRENT_YEAR_NUM = new Date().getFullYear();
const CURRENT_YEAR = CURRENT_YEAR_NUM.toString();

const SPDX_RE = /SPDX-License-Identifier:\s*\S+/;
const EXCLUDED_DIRS = new Set(['node_modules', 'cdk.out', '.git', 'vendor', 'venv', '.venv', 'dist']);

// Year of the most recent commit that touched rel (relative to ROOT), or 0 if
// git is unavailable or the file has never been committed. Callers treat 0 as
// the current year, so new/uncommitted files must carry the current year.
function gitLastCommitYear(rel: string): number {
  const res = spawnSync(
    'git',
    ['log', '--follow', '-1', '--format=%ad', '--date=format:%Y', '--', rel],
    { cwd: ROOT, encoding: 'utf-8' },
  );
  if (res.error || res.status !== 0) return 0;
  const year = parseInt(res.stdout.trim(), 10);
  return Number.isNaN(year) ? 0 : year;
}

// The configured copyright holder, from `.copyright-holder` (preferred, committed)
// or the COPYRIGHT_HOLDER env var. Returns null when unset/placeholder.
function expectedHolder(): string | null {
  let fromFile = '';
  try {
    fromFile = fs.readFileSync(path.join(ROOT, '.copyright-holder'), 'utf-8').trim();
  } catch { /* no file yet */ }
  const holder = (fromFile || (process.env.COPYRIGHT_HOLDER ?? '')).trim();
  return holder || null;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function firstLines(filePath: string): string[] {
  return fs.readFileSync(filePath, 'utf-8').split('\n').slice(0, 5);
}

function hasSpdx(filePath: string): boolean {
  return firstLines(filePath).some(l => SPDX_RE.test(l));
}

// Presence only: any year or year-range followed by the holder. The year's
// currency is checked separately against the file's last git-commit year, so a
// dormant prior-year file isn't forced to bump on every Jan 1.
function hasHolderHeader(filePath: string, holder: string): boolean {
  const re = new RegExp(`Copyright \\(c\\) \\d{4}(?:-\\d{4})?\\s+${escapeRegExp(holder)}`);
  return firstLines(filePath).some(l => re.test(l));
}

function collectFiles(dir: string, ext: string): string[] {
  const results: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (EXCLUDED_DIRS.has(entry.name)) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...collectFiles(full, ext));
    } else if (entry.name.endsWith(ext) && !entry.name.endsWith('.d.ts')) {
      results.push(full);
    }
  }
  return results;
}

function scriptFiles(): string[] {
  const scriptsDir = path.join(ROOT, 'scripts');
  if (!fs.existsSync(scriptsDir)) return [];
  return fs.readdirSync(scriptsDir)
    .filter(f => f.endsWith('.sh'))
    .map(f => path.join(scriptsDir, f));
}

const rel = (f: string) => path.relative(ROOT, f);

// Enforce: every non-SPDX file in the group carries the configured holder's
// current-year copyright header.
function checkGroup(files: string[], label: string): void {
  const nonExempt = files.filter(f => !hasSpdx(f)); // SPDX files are exempt
  const holder = expectedHolder();

  if (holder === null) {
    if (nonExempt.length === 0) return; // nothing of yours to enforce yet
    throw new Error(
      `Copyright holder not set. Create a .copyright-holder file (or run ./setup_dev.sh), ` +
      `then add\n  Copyright (c) ${CURRENT_YEAR} <holder>\n` +
      `to these ${label} files (or give them an SPDX identifier):\n` +
      nonExempt.map(rel).join('\n'),
    );
  }

  const violations: string[] = [];
  for (const f of nonExempt) {
    if (!hasHolderHeader(f, holder)) {
      violations.push(`${rel(f)}: missing a \`Copyright (c) <year> ${holder}\` header or SPDX tag`);
      continue;
    }
    // Require the current year only when the file was last committed this year,
    // or when its commit year is unknown (new/uncommitted/git unavailable).
    const header = firstLines(f).join('\n');
    const lastYear = gitLastCommitYear(rel(f));
    if ((lastYear === 0 || lastYear === CURRENT_YEAR_NUM) && !header.includes(CURRENT_YEAR)) {
      violations.push(`${rel(f)}: copyright header must include ${CURRENT_YEAR} (file changed this year)`);
    }
  }
  expect(violations).toEqual([]);
}

test('TypeScript files carry an SPDX tag or the project copyright header', () => {
  checkGroup(collectFiles(ROOT, '.ts'), 'TypeScript');
});

test('shell scripts carry an SPDX tag or the project copyright header', () => {
  checkGroup(scriptFiles(), 'shell');
});
