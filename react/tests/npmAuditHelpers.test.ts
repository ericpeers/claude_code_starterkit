// SPDX-License-Identifier: MIT
import { describe, it, expect } from '@jest/globals';
import {
  classifyFixable,
  compareVersions,
  describeAdvisories,
  describeFix,
  installedVersion,
  isDowngrade,
  type AuditVulnerability,
  type FixableVulnerability,
} from './helpers/npmAudit';
import { ROOT } from './helpers/repoFiles';

describe('compareVersions', () => {
  it('orders by major, then minor, then patch', () => {
    expect(compareVersions('29.1.2', '29.4.12')).toBe(-1);
    expect(compareVersions('30.0.0', '29.9.9')).toBe(1);
    expect(compareVersions('7.11.0', '7.11.0')).toBe(0);
  });

  it('compares numerically, not lexically', () => {
    expect(compareVersions('1.10.0', '1.9.0')).toBe(1);
  });

  it('ignores prerelease tags', () => {
    expect(compareVersions('2.0.0-beta.1', '2.0.0')).toBe(0);
  });

  it('treats missing segments as zero', () => {
    expect(compareVersions('25', '25.0.0')).toBe(0);
  });
});

describe('isDowngrade', () => {
  it('flags a fix older than the installed version', () => {
    expect(isDowngrade('25.0.0', '30.5.2')).toBe(true);
  });

  it('accepts a fix newer than the installed version', () => {
    expect(isDowngrade('8.3.0', '7.18.1')).toBe(false);
  });

  it('accepts a fix equal to the installed version', () => {
    expect(isDowngrade('1.2.3', '1.2.3')).toBe(false);
  });

  it('does not flag when the installed version is unknown', () => {
    expect(isDowngrade('1.0.0', undefined)).toBe(false);
  });
});

describe('installedVersion', () => {
  it('reads the version of an installed package', () => {
    expect(installedVersion(ROOT, 'jest')).toMatch(/^\d+\.\d+\.\d+/);
  });

  it('returns undefined for a package that is not installed', () => {
    expect(installedVersion(ROOT, 'no-such-package-xyz')).toBeUndefined();
  });
});

describe('describeFix', () => {
  const jestFix: FixableVulnerability = {
    name: 'deepmerge',
    severity: 'low',
    fixAvailable: { name: 'jest', version: '25.0.0', isSemVerMajor: true },
  };

  it('tells the reader to run npm audit fix when npm gives no target', () => {
    const vuln: FixableVulnerability = { name: 'lodash', severity: 'low', fixAvailable: true };
    expect(describeFix(vuln, undefined)).toEqual({
      kind: 'forward',
      line: '  - lodash (low) — run npm audit fix',
    });
  });

  it('classifies a fix older than the installed version as a downgrade', () => {
    expect(describeFix(jestFix, '30.5.2')).toEqual({
      kind: 'downgrade',
      line: '  - deepmerge (low) — npm offers jest@25.0.0, a downgrade from 30.5.2',
    });
  });

  it('marks a newer major fix as a forward upgrade with a (major) suffix', () => {
    expect(describeFix(jestFix, '24.0.0')).toEqual({
      kind: 'forward',
      line: '  - deepmerge (low) — upgrade to jest@25.0.0 (major)',
    });
  });

  it('omits the suffix for a newer minor fix', () => {
    const vuln: FixableVulnerability = {
      name: 'vite',
      severity: 'moderate',
      fixAvailable: { name: 'vite', version: '7.2.0', isSemVerMajor: false },
    };
    expect(describeFix(vuln, '7.1.0').line).toBe('  - vite (moderate) — upgrade to vite@7.2.0');
  });

  it('reports a forward fix when the installed version is unknown', () => {
    expect(describeFix(jestFix, undefined).kind).toBe('forward');
  });
});

describe('describeAdvisories', () => {
  const advisory = {
    title: 'ip-address: zone identifier matched in quadratic time',
    url: 'https://github.com/advisories/GHSA-rxp7-6hxx-x8q5',
    range: '<=10.7.3',
  };

  it('names the advisory, its affected range, the installed version and the link', () => {
    expect(describeAdvisories([advisory], '10.7.2')).toEqual([
      '      ip-address: zone identifier matched in quadratic time (affects <=10.7.3, installed 10.7.2)\n' +
        '        https://github.com/advisories/GHSA-rxp7-6hxx-x8q5',
    ]);
  });

  it('leaves the installed version out when it is unknown', () => {
    expect(describeAdvisories([advisory], undefined)[0]).toContain('(affects <=10.7.3)');
  });

  it('names the dependency when the package is flagged through another package', () => {
    expect(describeAdvisories(['ip-address'], '2.8.10')).toEqual([
      '      via vulnerable dependency ip-address',
    ]);
  });

  it('returns no lines for an empty via list', () => {
    expect(describeAdvisories([], '1.0.0')).toEqual([]);
  });
});

function fixtureVersion(name: string): string | undefined {
  return { jest: '30.5.2', vite: '7.1.0' }[name];
}

describe('classifyFixable', () => {
  const vulns: AuditVulnerability[] = [
    { name: 'vite', severity: 'moderate', isDirect: true, via: [],
      fixAvailable: { name: 'vite', version: '7.2.0', isSemVerMajor: false } },
    { name: 'jest-config', severity: 'low', isDirect: false, via: ['deepmerge'],
      fixAvailable: { name: 'jest', version: '25.1.0', isSemVerMajor: true } },
    { name: 'left-pad', severity: 'high', isDirect: false, via: [], fixAvailable: false },
  ];

  it('detects a forward fix and a downgrade, and skips the unfixable entry', () => {
    expect(classifyFixable(vulns, new Set(), fixtureVersion)).toEqual({
      forward: ['  - vite (moderate) — upgrade to vite@7.2.0'],
      downgrades: [
        '  - jest-config (low) — npm offers jest@25.1.0, a downgrade from 30.5.2\n' +
          '      via vulnerable dependency deepmerge',
      ],
    });
  });

  it('leaves out a package on the no-safe-fix list', () => {
    expect(classifyFixable(vulns, new Set(['jest-config']), fixtureVersion).downgrades).toEqual([]);
  });
});
