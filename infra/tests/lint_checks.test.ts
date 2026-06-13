// SPDX-License-Identifier: MIT
//
// Shell-script quality gates. Generic and reusable: shellcheck cleanliness,
// strict-mode enforcement, required-env-var guards, and (for AWS projects) a
// --region presence check.

import * as fs from 'fs';
import * as path from 'path';
import { spawnSync } from 'child_process';

const REPO_ROOT = path.resolve(__dirname, '..');
const SCRIPTS_DIR = path.join(REPO_ROOT, 'scripts');

// Every .sh under scripts/ plus any at the repo root (e.g. setup_dev.sh).
const shFiles = (dir: string) =>
  fs.existsSync(dir)
    ? fs.readdirSync(dir).filter(f => f.endsWith('.sh')).map(f => path.join(dir, f))
    : [];

const SCRIPT_FILES = [...shFiles(SCRIPTS_DIR), ...shFiles(REPO_ROOT)].sort();

// ── ShellCheck ────────────────────────────────────────────────────────────────

test('shellcheck passes on all scripts', () => {
  if (SCRIPT_FILES.length === 0) return;
  const which = spawnSync('which', ['shellcheck'], { encoding: 'utf-8' });
  if (which.status !== 0) {
    console.log('  shellcheck not installed, skipping');
    return;
  }
  const result = spawnSync('shellcheck', ['-x', ...SCRIPT_FILES], { encoding: 'utf-8' });
  if (result.status !== 0) {
    throw new Error(`shellcheck found issues:\n${result.stdout}${result.stderr}`);
  }
});

// ── Strict mode ────────────────────────────────────────────────────────────────

test('every script has set -euo pipefail', () => {
  const missing = SCRIPT_FILES.filter(
    f => !fs.readFileSync(f, 'utf-8').includes('set -euo pipefail'),
  );
  if (missing.length > 0) {
    throw new Error(
      `Missing 'set -euo pipefail' in:\n${missing.map(f => `  ${path.basename(f)}`).join('\n')}`,
    );
  }
});

// ── Required env-var guards ─────────────────────────────────────────────────────

test('scripts guard their required env vars with ${VAR:?...}', () => {
  // A script that depends on an env var should assert it is set via the
  // ${VAR:?message} guard, so a missing var fails fast with a clear message
  // instead of a confusing downstream error.
  //
  // EXAMPLE (empty by default so a fresh scaffold passes). Fill in per script:
  //   'deploy.sh': ['TARGET_HOST', 'SSH_KEY'],
  const REQUIRED: Record<string, string[]> = {};

  const violations: string[] = [];
  for (const [name, vars] of Object.entries(REQUIRED)) {
    const file = path.join(SCRIPTS_DIR, name);
    if (!fs.existsSync(file)) {
      violations.push(`${name}: file not found`);
      continue;
    }
    const content = fs.readFileSync(file, 'utf-8');
    for (const v of vars) {
      if (!new RegExp(`\\$\\{${v}:\\?`).test(content)) {
        violations.push(`${name}: missing \${${v}:?...} guard`);
      }
    }
  }
  if (violations.length > 0) {
    throw new Error(`Env-var guards missing:\n${violations.map(v => `  ${v}`).join('\n')}`);
  }
});

// ── AWS --region presence (drop this test for non-AWS projects) ─────────────────

test('every aws CLI call specifies --region', () => {
  // setup_dev.sh bootstraps credentials before any region context exists, so its
  // 'aws configure' / 'aws sts get-caller-identity' calls are region-independent.
  const SKIP = new Set(['setup_dev.sh']);

  const violations: string[] = [];
  for (const file of SCRIPT_FILES) {
    if (SKIP.has(path.basename(file))) continue;
    for (const block of commandBlocks(fs.readFileSync(file, 'utf-8'))) {
      // Anchor to command positions so 'aws' inside echo strings doesn't trip it.
      const awsInvocation =
        /(?:^|[;|&$(\n]\s*|(?:exec|sudo)\s+)aws\s+(s3|ssm|ec2|rds|iam|cloudfront|cloudformation|secretsmanager|sts)\b/;
      if (awsInvocation.test(block) && !block.includes('--region')) {
        violations.push(`${path.basename(file)}: ${block.trim().slice(0, 100)}`);
      }
    }
  }
  if (violations.length > 0) {
    throw new Error(`aws CLI calls missing --region:\n${violations.map(v => `  ${v}`).join('\n')}`);
  }
});

// ── Helpers ─────────────────────────────────────────────────────────────────────

/**
 * Join backslash-continued lines into logical command blocks, skipping comments.
 * Mirrors how bash parses multi-line commands so the --region check works whether
 * the flag is on the same line as 'aws' or on a continuation.
 */
function commandBlocks(content: string): string[] {
  const blocks: string[] = [];
  let current = '';
  for (const line of content.split('\n')) {
    const trimmed = line.trimEnd();
    if (trimmed.trimStart().startsWith('#')) {
      if (current) { blocks.push(current); current = ''; }
      continue;
    }
    if (trimmed.endsWith('\\')) {
      current += trimmed.slice(0, -1) + ' ';
    } else {
      current += trimmed;
      if (current.trim()) blocks.push(current);
      current = '';
    }
  }
  if (current.trim()) blocks.push(current);
  return blocks;
}
