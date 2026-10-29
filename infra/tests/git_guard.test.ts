// SPDX-License-Identifier: MIT
//
// Tests run only read-only git. CLAUDE.md gives the reason.

import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import gitGuardSetup, { gitEnvVars, gitWriteReasons } from './helpers/gitGuard';

const TEST_DIR = __dirname;
const SELF = path.basename(__filename);
const GUARD_COPY = path.join(TEST_DIR, 'helpers', 'gitGuard.ts');
// The kit's source for GUARD_COPY. A scaffolded project has none.
const KIT_GUARD = path.join(TEST_DIR, '..', '..', 'shared', 'tests', 'gitGuard.ts');

// ── Source scan ───────────────────────────────────────────────────────────────

/** A line for each git write in one test file. */
function fileViolations(file: string): string[] {
  const source = fs.readFileSync(path.join(TEST_DIR, file), 'utf-8');
  return gitWriteReasons(source).map(reason => `  ${file}: ${reason}`); // the closure carries file
}

/** A test source other than this file, whose fixtures name writes on purpose. */
function isScannedTest(file: string): boolean {
  return file.endsWith('.ts') && !file.endsWith('.d.ts') && file !== SELF;
}

/** Every file under tests/, relative to it. */
function testFiles(): string[] {
  return fs.readdirSync(TEST_DIR, { recursive: true, encoding: 'utf-8' });
}

test.each<[string, boolean]>([
  ['git -C "$dir" log --oneline', false],
  ['git --no-pager log -1 foo', false],
  ['git -C "$dir" status --porcelain', true],
  ['git -c user.name=t -c user.email=t@t commit -q -m x', true],
  ['git -c alias.log=!sh log', true],
  ['git init -q --bare origin.git', true],
  ['git --git-dir=x push origin HEAD', true],
  ['git filter-branch --all', true],
  ["spawnSync('git', ['-C', repo, 'init', '-q'])", true],
  ["spawnSync('git', ['ls-files', '-z'])", false],
  ["spawnSync('git', args)", true],
  ["git('config', 'user.email', 'a@b')", true],
  ['a tree git cannot read stops the deploy', false],
  ['refuses --no-git for prod', false],
])('the scan flags %j: %s', (source, flagged) => {
  expect(gitWriteReasons(source).length > 0).toBe(flagged);
});

test.each([
  "spawnSync('git', ['init'])",
  "git('config', 'user.email', 'a@b')",
])('the scan reports %j once', (source) => {
  expect(gitWriteReasons(source)).toHaveLength(1);
});

test('no test file runs a git write', () => {
  const violations = testFiles().filter(isScannedTest).flatMap(fileViolations);
  if (violations.length) {
    throw new Error(`Tests run only read-only git; stub git instead:\n${violations.join('\n')}`);
  }
});

// Runs only inside the kit, where the shared source exists.
(fs.existsSync(KIT_GUARD) ? test : test.skip)('the guard matches the kit copy', () => {
  expect(fs.readFileSync(GUARD_COPY, 'utf-8')).toBe(fs.readFileSync(KIT_GUARD, 'utf-8'));
});

// ── Hook variables ────────────────────────────────────────────────────────────

// Runs before the setup test below, which calls gitGuardSetup itself.
test('no git hook variable reaches the tests', () => {
  expect(gitEnvVars()).toEqual(['GIT_CEILING_DIRECTORIES']);
});

test('the global setup clears the git hook variables', () => {
  process.env.GIT_DIR = '/repo/.git';
  process.env.GIT_INDEX_FILE = '/repo/.git/index';
  gitGuardSetup();
  expect(gitEnvVars()).toEqual(['GIT_CEILING_DIRECTORIES']);
  expect(process.env.GIT_CEILING_DIRECTORIES).toBe(os.tmpdir());
});
