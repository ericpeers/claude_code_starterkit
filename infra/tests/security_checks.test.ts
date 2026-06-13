// SPDX-License-Identifier: MIT
//
// Security gates: dependency audit + a conservative hardcoded-secret scan.

import * as fs from 'fs';
import * as path from 'path';
import { spawnSync } from 'child_process';

const ROOT_DIR = path.resolve(__dirname, '..');

// ── npm audit ─────────────────────────────────────────────────────────────────

test('npm audit: no high or critical vulnerabilities', () => {
  const result = spawnSync('npm', ['audit', '--audit-level=high'], {
    encoding: 'utf-8',
    cwd: ROOT_DIR,
  });
  if (result.status !== 0) {
    throw new Error(`npm audit found high/critical vulnerabilities:\n${result.stdout}${result.stderr}`);
  }
}, 30000); // network request — allow up to 30s

// ── Hardcoded-secret scan ───────────────────────────────────────────────────────

// High-confidence patterns only, to avoid false positives. Add your own as needed.
const SECRET_PATTERNS: { name: string; re: RegExp }[] = [
  { name: 'AWS access key id', re: /\bAKIA[0-9A-Z]{16}\b/ },
  { name: 'private key block', re: /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/ },
];

const SCAN_EXTS = new Set(['.ts', '.js', '.sh', '.json', '.yaml', '.yml']);
const EXCLUDED_DIRS = new Set(['node_modules', 'cdk.out', '.git', 'dist', '.venv', 'venv']);
// Sample/template files legitimately show placeholder shapes, not real secrets.
const EXCLUDED_FILES = new Set(['.env_sample', 'package-lock.json']);

function collectScannable(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (EXCLUDED_DIRS.has(entry.name) || EXCLUDED_FILES.has(entry.name)) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...collectScannable(full));
    } else if (SCAN_EXTS.has(path.extname(entry.name))) {
      out.push(full);
    }
  }
  return out;
}

test('no hardcoded secrets in tracked source', () => {
  const findings: string[] = [];
  for (const file of collectScannable(ROOT_DIR)) {
    const content = fs.readFileSync(file, 'utf-8');
    for (const { name, re } of SECRET_PATTERNS) {
      if (re.test(content)) {
        findings.push(`${path.relative(ROOT_DIR, file)}: possible ${name}`);
      }
    }
  }
  expect(findings).toEqual([]);
});
