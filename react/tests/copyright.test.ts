// SPDX-License-Identifier: MIT
//
// License / copyright header gate for TypeScript sources.
//
// Two kinds of files coexist in a project built from this kit:
//   * Kit-supplied boilerplate declares `SPDX-License-Identifier: MIT` and is
//     EXEMPT here. It carries no personal copyright holder by design.
//   * Your own source must carry a current-year copyright header naming the
//     project's configured holder (the .copyright-holder file written by
//     setup_dev.sh). This is how you enforce your ownership on the code you write.
//
// A fresh scaffold is green (everything shipped is SPDX-exempt). The gate starts
// enforcing once you add your own .ts/.tsx files without an SPDX tag or header.
import { describe, it, expect } from '@jest/globals';
import * as fs from 'fs';
import * as path from 'path';
import { spawnSync } from 'child_process';

const ROOT = process.cwd();
const CURRENT_YEAR = new Date().getFullYear();
const YEAR = CURRENT_YEAR.toString();
const SPDX_RE = /SPDX-License-Identifier:\s*\S+/;
const EXCLUDED = new Set(['node_modules', '.git', 'dist', 'coverage', '.venv', 'build']);

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

function expectedHolder(): string | null {
  try {
    const h = fs.readFileSync(path.join(ROOT, '.copyright-holder'), 'utf-8').trim();
    if (h) return h;
  } catch { /* no file yet */ }
  return (process.env.COPYRIGHT_HOLDER ?? '').trim() || null;
}

function tsFiles(dir: string): string[] {
  const out: string[] = [];
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    if (EXCLUDED.has(ent.name)) continue;
    const full = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      out.push(...tsFiles(full));
    } else if ((ent.name.endsWith('.ts') || ent.name.endsWith('.tsx')) && !ent.name.endsWith('.d.ts')) {
      out.push(full);
    }
  }
  return out;
}

function head(filePath: string): string {
  return fs.readFileSync(filePath, 'utf-8').split('\n').slice(0, 5).join('\n');
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

describe('license headers', () => {
  it('every .ts/.tsx file has an SPDX tag or a current copyright header', () => {
    const holder = expectedHolder();
    const nonExempt = tsFiles(ROOT).filter(f => !SPDX_RE.test(head(f)));
    const rel = (f: string) => path.relative(ROOT, f);

    if (holder === null) {
      if (nonExempt.length === 0) return; // nothing of yours to enforce yet
      throw new Error(
        `Copyright holder not set. Create .copyright-holder (or run ./setup_dev.sh), then add\n` +
        `  // Copyright (c) ${YEAR} <holder>\n` +
        `to these files (or give them an SPDX identifier):\n` +
        nonExempt.map(rel).join('\n'),
      );
    }

    // Presence only: any year or year-range followed by the holder. The year's
    // currency is checked separately against the file's last git-commit year, so
    // a dormant prior-year file isn't forced to bump on every Jan 1.
    const presenceRe = new RegExp(`Copyright \\(c\\) \\d{4}(?:-\\d{4})?\\s+${escapeRegExp(holder)}`);
    const violations: string[] = [];
    for (const f of nonExempt) {
      const h = head(f);
      if (!presenceRe.test(h)) {
        violations.push(`${rel(f)}: missing a \`Copyright (c) <year> ${holder}\` header or SPDX tag`);
        continue;
      }
      // Require the current year only when the file was last committed this year,
      // or when its commit year is unknown (new/uncommitted/git unavailable).
      const lastYear = gitLastCommitYear(rel(f));
      if ((lastYear === 0 || lastYear === CURRENT_YEAR) && !h.includes(YEAR)) {
        violations.push(`${rel(f)}: copyright header must include ${YEAR} (file changed this year)`);
      }
    }
    expect(violations).toEqual([]);
  });
});
