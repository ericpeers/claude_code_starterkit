/**
 * @jest-environment node
 * SPDX-License-Identifier: MIT
 */
import { describe, it, expect } from '@jest/globals';
import { init } from 'license-checker-rseidelsohn';
import type { ModuleInfos } from 'license-checker-rseidelsohn';

// Permissive licenses safe to ship in a commercial product. Add to this set
// only after confirming a license is genuinely compatible with your distribution
// model — do not add copyleft licenses (GPL, AGPL, LGPL) without legal review.
const ALLOWED_LICENSES = new Set([
  'MIT',
  'MIT-0',        // MIT without attribution requirement
  'Apache-2.0',
  'ISC',
  'BSD-2-Clause',
  'BSD-3-Clause',
  '0BSD',
  'CC0-1.0',
  'CC-BY-3.0',    // data/content packages
  'CC-BY-4.0',    // data/content packages (e.g. caniuse-lite)
  'MPL-2.0',      // weak copyleft; build-time tools only, not distributed in app
  'Unlicense',
  'Python-2.0',
  'BlueOak-1.0.0',
  'OFL-1.1',      // SIL Open Font License — bundled fonts
]);

function toTokens(licenses: string | string[] | undefined): string[] {
  if (!licenses) return ['UNKNOWN'];
  const raw = Array.isArray(licenses) ? licenses : [licenses];
  return raw.flatMap((l) =>
    l
      .replace(/[()]/g, '')
      .split(/\s+(?:OR|AND)\s+|,\s*/)
      .map((s) => s.trim())
      .filter(Boolean),
  );
}

function scanPackages(): Promise<ModuleInfos> {
  return new Promise((resolve, reject) => {
    init(
      { start: process.cwd(), excludePrivatePackages: true },
      (err: Error, packages: ModuleInfos) => (err ? reject(err) : resolve(packages)),
    );
  });
}

describe('dependency licenses', () => {
  it('all packages use commercially permissive licenses', async () => {
    const packages = await scanPackages();

    const violations = Object.entries(packages)
      .filter(([, info]) => {
        const tokens = toTokens(info.licenses);
        return tokens.some((t) => !ALLOWED_LICENSES.has(t));
      })
      .map(([name, info]) => `${name}: ${JSON.stringify(info.licenses)}`);

    expect(violations).toEqual([]);
  });
});
