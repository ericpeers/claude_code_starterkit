// SPDX-License-Identifier: MIT
import { describe, it, expect } from '@jest/globals';
import { existsSync, readFileSync } from 'fs';
import { tmpdir } from 'os';
import * as path from 'path';
import { repoFiles, REPO_SKIP_DIRS, ROOT } from './helpers/repoFiles';
import gitGuardSetup, { gitEnvVars, gitWriteReasons } from './helpers/gitGuard';

// Tests run only read-only git. CLAUDE.md gives the reason.

const SELF = 'tests/gitGuard.test.ts';
const GUARD_COPY = 'tests/helpers/gitGuard.ts';
// The kit's source for GUARD_COPY. A scaffolded project has none.
const KIT_GUARD = path.join(ROOT, '..', 'shared', 'tests', 'gitGuard.ts');
// Project gates, unit tests with their helpers, and the e2e specs.
const TEST_DIRS = ['tests', 'src/__tests__', 'e2e'];

/** A line for each git write in one test file. */
function fileViolations(rel: string): string[] {
  const source = readFileSync(path.join(ROOT, rel), 'utf8');
  return gitWriteReasons(source).map(reason => `  ${rel}: ${reason}`); // the closure carries rel
}

/** Whether rel is a test source to scan. This file is skipped because its fixtures name writes. */
function isScannedSource(rel: string): boolean {
  return /\.(ts|tsx|js|mjs|cjs)$/.test(rel) && !rel.endsWith('.d.ts') && rel !== SELF;
}

/** The repo files under one test directory. A missing directory has none. */
function dirFiles(dir: string): string[] {
  return [...repoFiles(ROOT, REPO_SKIP_DIRS, path.join(ROOT, dir))];
}

/** Every test source under TEST_DIRS. */
function scannedSources(): string[] {
  return TEST_DIRS.flatMap(dirFiles).filter(isScannedSource);
}

describe('git write scan', () => {
  it.each<[string, boolean]>([
    ['git -C "$dir" log --oneline', false],
    ['git --no-pager log -1 foo', false],
    ['git -C "$dir" status --porcelain', true],
    ['git -c user.name=t -c user.email=t@t commit -q -m x', true],
    ['git -c alias.log=!sh log', true],
    ['git init -q --bare origin.git', true],
    ['git --git-dir=x push origin HEAD', true],
    ['git filter-branch --all', true],
    ['git reflog expire --all', true],
    ["spawnSync('git', ['-C', repo, 'init', '-q'])", true],
    ["spawnSync('git', ['ls-files', '-z'])", false],
    ["spawnSync('git', ['--config-env=core.hooksPath=X', 'log'])", true],
    ["spawnSync('git', [sub])", true],
    ["spawnSync('git', args)", true],
    ["git('config', 'user.email', 'a@b')", true],
    ["execSync('git log --name-only --format=@%ad')", false],
    ['skips the prepare hook (not a git repo here)', false],
    ['', false],
  ])('flags %j: %s', (source, flagged) => {
    expect(gitWriteReasons(source).length > 0).toBe(flagged);
  });

  it.each([
    "spawnSync('git', ['init'])",
    "git('config', 'user.email', 'a@b')",
  ])('reports %j once', (source) => {
    expect(gitWriteReasons(source)).toHaveLength(1);
  });

  it('scans the project gates, their helpers and the e2e specs', () => {
    const sources = scannedSources();
    expect(sources).toEqual(expect.arrayContaining([
      'tests/copyright.test.ts',
      GUARD_COPY,
      'e2e/git-guard.spec.ts',
    ]));
    expect(sources).not.toContain(SELF);
  });
});

describe('tests run only read-only git', () => {
  it('no test file runs a git write', () => {
    const violations = scannedSources().flatMap(fileViolations);
    if (violations.length) {
      throw new Error(`Tests run only read-only git. Stub git instead:\n${violations.join('\n')}`);
    }
  });

  it('no git hook variable reaches the tests', () => {
    expect(gitEnvVars()).toEqual(['GIT_CEILING_DIRECTORIES']);
  });
});

describe('gitGuardSetup', () => {
  it('strips the variables git exports to hooks', () => {
    process.env.GIT_DIR = '/repo/.git';
    process.env.GIT_INDEX_FILE = '/repo/.git/index';
    gitGuardSetup();
    expect(gitEnvVars()).toEqual(['GIT_CEILING_DIRECTORIES']);
  });

  it('stops git discovery at the temp dir', () => {
    process.env.GIT_CEILING_DIRECTORIES = '/elsewhere';
    gitGuardSetup();
    expect(process.env.GIT_CEILING_DIRECTORIES).toBe(tmpdir());
  });
});

// Runs only inside the kit, where the shared source exists.
(existsSync(KIT_GUARD) ? it : it.skip)('the guard matches the kit copy', () => {
  expect(readFileSync(path.join(ROOT, GUARD_COPY), 'utf8')).toBe(readFileSync(KIT_GUARD, 'utf8'));
});
