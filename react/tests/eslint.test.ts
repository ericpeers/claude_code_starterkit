// SPDX-License-Identifier: MIT
import { describe, it, expect } from '@jest/globals';
import { spawnSync } from 'child_process';
import * as path from 'path';
import { ROOT } from './helpers/repoFiles';

// Lint errors accepted as technical debt, as "path:ruleId" keys. Add an entry only
// when the fix needs a large refactor.
const ACCEPTED_ERRORS = new Set<string>([]);

interface LintMessage {
  ruleId: string | null;
  severity: number;
  message: string;
  line: number;
  column: number;
}

interface LintResult {
  filePath: string;
  messages: LintMessage[];
}

/** Runs ESLint with JSON output. input, when given, is linted as stdin. */
function runEslint(args: string[], input?: string): LintResult[] {
  // The content strategy keys the cache on file hashes, so a checkout or touch keeps it warm.
  const result = spawnSync(
    'npx',
    ['eslint', '--format', 'json', '--cache', '--cache-strategy', 'content',
      '--cache-location', 'node_modules/.cache/eslint/', ...args],
    { cwd: ROOT, encoding: 'utf8', timeout: 60_000, input },
  );
  if (result.error) throw result.error;
  try {
    return JSON.parse(result.stdout) as LintResult[];
  } catch {
    throw new Error(`Failed to parse ESLint output:\n${result.stdout}\n${result.stderr}`);
  }
}

function relPath(filePath: string): string {
  return path.relative(ROOT, filePath).split(path.sep).join('/');
}

/** One line per error-severity message whose "path:ruleId" key is not in accepted. */
function unacceptedErrors(results: LintResult[], accepted: ReadonlySet<string>): string[] {
  const out: string[] = [];
  for (const file of results) {
    const rel = relPath(file.filePath);
    for (const msg of file.messages) {
      if (msg.severity < 2) continue;
      if (accepted.has(`${rel}:${msg.ruleId ?? 'unknown'}`)) continue;
      out.push(`  ${rel}:${msg.line}:${msg.column} — ${msg.ruleId}: ${msg.message}`);
    }
  }
  return out;
}

describe('ESLint', () => {
  it('has no lint errors outside the accepted list', () => {
    const unaccepted = unacceptedErrors(runEslint(['.']), ACCEPTED_ERRORS);
    if (unaccepted.length > 0) {
      throw new Error(`ESLint errors found:\n${unaccepted.join('\n')}`);
    }
  });

  // A config that lints nothing would pass the gate above, so a fixture with a known
  // error must come back reported.
  it('detects an error in a fixture', () => {
    const results = runEslint(['--stdin', '--stdin-filename', 'src/lintProbe.ts'], 'const unused = 1;\nexport {};\n');
    expect(unacceptedErrors(results, ACCEPTED_ERRORS)).toEqual([
      expect.stringContaining('src/lintProbe.ts:1:7 — @typescript-eslint/no-unused-vars'),
    ]);
  });

  it('skips an error on the accepted list', () => {
    const results: LintResult[] = [{
      filePath: path.join(ROOT, 'src/a.ts'),
      messages: [{ ruleId: 'no-console', severity: 2, message: 'x', line: 1, column: 1 }],
    }];
    expect(unacceptedErrors(results, new Set(['src/a.ts:no-console']))).toEqual([]);
    expect(unacceptedErrors(results, new Set())).toHaveLength(1);
  });
});
