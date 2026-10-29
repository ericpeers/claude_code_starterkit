// SPDX-License-Identifier: MIT
import { describe, it, expect } from '@jest/globals';
import { spawnSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { REPO_SKIP_DIRS, ROOT, repoFiles } from './helpers/repoFiles';

/** Dead code reviewed and accepted until a date, as an id in the knip-derived format below. */
interface DeadCodeAllowance {
  id: string;
  until: string;
  reason: string;
}

// Code confirmed uncalled, kept until a YYYY-MM-DD date about a month out. Example:
//   { id: 'export:src/foo.ts:bar', until: '2026-08-15', reason: 'blocked on X.' }
const ACCEPTED_DEAD_CODE: DeadCodeAllowance[] = [];

/** Dead code a fresh scaffold ships, accepted while whileMissing does not exist. */
interface ScaffoldAllowance {
  id: string;
  whileMissing: string;
}

// The app's entry file and its first component test import these packages.
const SCAFFOLD_DEAD_CODE: ScaffoldAllowance[] = [
  { id: 'dependency:react', whileMissing: 'src/main.tsx' },
  { id: 'dependency:react-dom', whileMissing: 'src/main.tsx' },
  { id: 'devDependency:@types/react', whileMissing: 'src/main.tsx' },
  { id: 'devDependency:@types/react-dom', whileMissing: 'src/main.tsx' },
  { id: 'devDependency:@testing-library/react', whileMissing: 'src/__tests__' },
  { id: 'devDependency:@testing-library/user-event', whileMissing: 'src/__tests__' },
];

// Only the fields this gate reads. Issue types: https://knip.dev/reference/issue-types
interface KnipIssueItem {
  name: string;
}
interface KnipFileReport {
  file: string;
  files: KnipIssueItem[];
  exports: KnipIssueItem[];
  types: KnipIssueItem[];
  dependencies: KnipIssueItem[];
  devDependencies: KnipIssueItem[];
}
interface KnipReport {
  issues: KnipFileReport[];
}

// Two tests read the report, so knip runs once.
let knipReport: KnipReport | undefined;

function runKnip(): KnipReport {
  knipReport ??= spawnKnip();
  return knipReport;
}

function spawnKnip(): KnipReport {
  const result = spawnSync(
    'node',
    ['./node_modules/knip/bin/knip.js', '--reporter', 'json', '--no-progress'],
    { cwd: ROOT, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 },
  );
  if (result.error) throw result.error;
  // knip exits non-zero when it finds issues, so only the output is checked.
  try {
    return JSON.parse(result.stdout) as KnipReport;
  } catch {
    throw new Error(
      `Failed to parse knip JSON output:\nstdout=${result.stdout.slice(0, 500)}\nstderr=${result.stderr.slice(0, 500)}`,
    );
  }
}

/** Flattens knip's per-file report into ids. An unused file is one id, not one per symbol. */
function collectDeadCodeIds(report: KnipReport): Set<string> {
  const ids = new Set<string>();
  for (const entry of report.issues) {
    for (const unusedFile of entry.files ?? []) ids.add(`file:${unusedFile.name}`);
    for (const unusedExport of entry.exports ?? []) ids.add(`export:${entry.file}:${unusedExport.name}`);
    for (const unusedType of entry.types ?? []) ids.add(`type:${entry.file}:${unusedType.name}`);
    for (const dependency of entry.dependencies ?? []) ids.add(`dependency:${dependency.name}`);
    for (const dependency of entry.devDependencies ?? []) ids.add(`devDependency:${dependency.name}`);
  }
  return ids;
}

/** Parses YYYY-MM-DD as midnight UTC. Other shapes throw, so a typo does not become an Invalid Date. */
function parseDate(text: string): Date {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(text)) {
    throw new Error(`Allowance \`until\` must be YYYY-MM-DD, got: ${JSON.stringify(text)}`);
  }
  return new Date(`${text}T00:00:00Z`);
}

/** The allowances whose until date is before now. */
function expiredAllowances(allowances: DeadCodeAllowance[], now: Date): DeadCodeAllowance[] {
  return allowances.filter(allowance => parseDate(allowance.until) < now); // the closure carries now
}

/** The id of an allowance. */
function allowanceId(allowance: { id: string }): string {
  return allowance.id;
}

/** One indented list line of an error message. */
function bullet(text: string): string {
  return `  - ${text}`;
}

/** An expired allowance as an error line. */
function formatExpired(allowance: DeadCodeAllowance): string {
  return bullet(`${allowance.id} (expired ${allowance.until})`);
}

function scaffoldEntryActive(entry: ScaffoldAllowance): boolean {
  return !fs.existsSync(path.join(ROOT, entry.whileMissing));
}

/** The ids accepted now: every dated allowance, and each scaffold entry whose file is missing. */
function acceptedIds(active: (entry: ScaffoldAllowance) => boolean): string[] {
  return [...ACCEPTED_DEAD_CODE.map(allowanceId), ...SCAFFOLD_DEAD_CODE.filter(active).map(allowanceId)];
}

/** The accepted ids knip no longer detects. */
function staleIds(accepted: string[], detected: Set<string>): string[] {
  return accepted.filter(id => !detected.has(id)); // the closure carries detected
}

/** The detected ids nobody accepted, sorted. */
function unacceptedIds(detected: Set<string>, accepted: string[]): string[] {
  const allowed = new Set(accepted);
  return [...detected].filter(id => !allowed.has(id)).sort(); // the closure carries allowed
}

interface SrcFile {
  rel: string;
  text: string;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * The .ts/.tsx files in files that no other file in files imports, skipping entries.
 * The match is on the basename, or the directory name for an index file.
 */
function orphanModules(files: SrcFile[], entries: ReadonlySet<string>): string[] {
  const orphans: string[] = [];
  for (const { rel } of files) {
    if (!/\.(ts|tsx)$/.test(rel) || rel.endsWith('.d.ts') || entries.has(rel)) continue;
    const base = path.basename(rel).replace(/\.(ts|tsx)$/, '');
    const needle = base === 'index' ? path.basename(path.dirname(rel)) : base;
    // Static and dynamic imports, on one line. A loose basename match can only hide an orphan.
    const pattern = new RegExp(`(from|import)[(\\t\\v\\f\\r ]+['"][^'"\\n]*/?${escapeRegExp(needle)}['"]`);
    // The closure carries rel and pattern.
    if (!files.some(other => other.rel !== rel && pattern.test(other.text))) orphans.push(rel);
  }
  return orphans;
}

/** The knip.json ignore list. knip accepts these files as intentional surface. */
function knipIgnored(): Set<string> {
  const config = JSON.parse(fs.readFileSync(path.join(ROOT, 'knip.json'), 'utf8')) as { ignore?: string[] };
  return new Set(config.ignore ?? []);
}

// Files loaded by tooling rather than by an import.
const TOOL_LOADED = new Set(['src/main.tsx', 'src/setupTests.ts', 'src/setupGlobals.ts']);

/** A file under src/ with its text. */
function readSrcFile(rel: string): SrcFile {
  return { rel, text: fs.readFileSync(path.join(ROOT, rel), 'utf8') };
}

/** Every file under src/ outside tests and mocks. */
function productionSources(): SrcFile[] {
  const skip = new Set([...REPO_SKIP_DIRS, '__tests__', '__mocks__']);
  return [...repoFiles(ROOT, skip, path.join(ROOT, 'src'))].map(readSrcFile);
}

describe('dead code', () => {
  it('has no entries past their until date', () => {
    const expired = expiredAllowances(ACCEPTED_DEAD_CODE, new Date());
    if (expired.length > 0) {
      const details = expired.map(formatExpired).join('\n');
      throw new Error(`Dead-code allowances expired. Fix the code, remove it, or extend the entry with a fresh reason:\n${details}`);
    }
  });

  it('has no allowances that knip no longer flags', () => {
    const stale = staleIds(acceptedIds(scaffoldEntryActive), collectDeadCodeIds(runKnip()));
    if (stale.length > 0) {
      throw new Error(`Stale dead-code allowances. The code was fixed or removed, so delete these entries:\n${stale.map(bullet).join('\n')}`);
    }
  });

  it('has no unaccepted dead code', () => {
    const unaccepted = unacceptedIds(collectDeadCodeIds(runKnip()), acceptedIds(scaffoldEntryActive));
    if (unaccepted.length > 0) {
      throw new Error(
        'Dead code detected. Fix it, or add an ACCEPTED_DEAD_CODE entry with a reason and a date about a month out:\n' +
          unaccepted.map(bullet).join('\n'),
      );
    }
  });

  // knip treats test files as entry points, so a module only its own test imports looks used.
  it('production modules have at least one non-test importer', () => {
    const entries = new Set([...TOOL_LOADED, ...knipIgnored()]);
    const orphans = orphanModules(productionSources(), entries);
    if (orphans.length > 0) {
      throw new Error(
        `Production modules imported only by their own tests:\n${orphans.map(bullet).join('\n')}\n\n` +
          'Delete the module and its test, or wire it to a production caller. Add a module that tooling ' +
          'loads to TOOL_LOADED.',
      );
    }
  });
});

function neverActive(): boolean {
  return false;
}

function alwaysActive(): boolean {
  return true;
}

describe('dead-code gate helpers', () => {
  it('flatten every knip issue kind into an id', () => {
    const report: KnipReport = {
      issues: [{
        file: 'src/a.ts',
        files: [{ name: 'src/a.ts' }],
        exports: [{ name: 'foo' }],
        types: [{ name: 'Bar' }],
        dependencies: [{ name: 'left-pad' }],
        devDependencies: [{ name: 'is-odd' }],
      }],
    };
    expect([...collectDeadCodeIds(report)].sort()).toEqual([
      'dependency:left-pad', 'devDependency:is-odd', 'export:src/a.ts:foo', 'file:src/a.ts', 'type:src/a.ts:Bar',
    ]);
  });

  it('detect an expired allowance and keep a current one', () => {
    const allowances = [
      { id: 'export:src/a.ts:old', until: '2026-05-31', reason: 'x' },
      { id: 'export:src/a.ts:today', until: '2026-06-01', reason: 'x' },
      { id: 'export:src/a.ts:new', until: '2026-06-02', reason: 'x' },
    ];
    const now = new Date('2026-06-01T00:00:00Z');
    expect(expiredAllowances(allowances, now).map(allowanceId)).toEqual(['export:src/a.ts:old']);
  });

  it('reject an until that is not YYYY-MM-DD', () => {
    expect(() => parseDate('2026-6-1')).toThrow('YYYY-MM-DD');
  });

  it('detect unaccepted and stale ids', () => {
    const detected = new Set(['export:src/a.ts:foo', 'dependency:react']);
    expect(unacceptedIds(detected, ['dependency:react'])).toEqual(['export:src/a.ts:foo']);
    expect(staleIds(['dependency:react', 'dependency:gone'], detected)).toEqual(['dependency:gone']);
  });

  it('accept scaffold entries only while their file is missing', () => {
    expect(acceptedIds(alwaysActive)).toContain('dependency:react');
    expect(acceptedIds(neverActive)).not.toContain('dependency:react');
  });

  it('detect a module only its own test imports', () => {
    const files: SrcFile[] = [
      { rel: 'src/main.tsx', text: "import { App } from './App';\nimport('@/pages/Lazy');" },
      { rel: 'src/App.tsx', text: "import { Widget } from '@/widgets';" },
      { rel: 'src/widgets/index.ts', text: "export { Widget } from './Widget';" },
      { rel: 'src/widgets/Widget.tsx', text: '' },
      { rel: 'src/pages/Lazy.tsx', text: '' },
      { rel: 'src/orphan.ts', text: '' },
    ];
    expect(orphanModules(files, new Set())).toEqual(['src/main.tsx', 'src/orphan.ts']);
  });
});
