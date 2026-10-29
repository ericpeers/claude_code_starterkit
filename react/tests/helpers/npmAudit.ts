// SPDX-License-Identifier: MIT
import * as fs from 'fs';
import * as path from 'path';

interface FixInfo {
  name: string;
  version: string;
  isSemVerMajor: boolean;
}

/** One `via` entry from `npm audit --json`: an advisory object, or the name of a vulnerable dependency. */
export type AuditVia = string | { title: string; url: string; range: string };

/** One entry of the `vulnerabilities` map in `npm audit --json`. */
export interface AuditVulnerability {
  name: string;
  severity: string;
  isDirect: boolean;
  via: AuditVia[];
  fixAvailable: boolean | FixInfo;
}

export interface FixableVulnerability {
  name: string;
  severity: string;
  fixAvailable: true | FixInfo;
}

export interface FixDescription {
  kind: 'forward' | 'downgrade';
  line: string;
}

/** A version segment as a number, or 0 when it is not numeric. */
function segmentNumber(segment: string): number {
  return Number(segment) || 0;
}

function numericParts(version: string): number[] {
  return version.split('-')[0].split('.').map(segmentNumber);
}

/** Compares the numeric major.minor.patch of two versions. Prerelease tags are ignored. */
export function compareVersions(versionA: string, versionB: string): number {
  const partsA = numericParts(versionA);
  const partsB = numericParts(versionB);
  for (let i = 0; i < 3; i++) {
    const diff = (partsA[i] ?? 0) - (partsB[i] ?? 0);
    if (diff !== 0) return Math.sign(diff);
  }
  return 0;
}

/** True when npm's offered fix installs an older version than the one in the tree. */
export function isDowngrade(fixVersion: string, installedVersion: string | undefined): boolean {
  return installedVersion !== undefined && compareVersions(fixVersion, installedVersion) < 0;
}

/** Version of the top-level install of name under root, or undefined if absent. */
export function installedVersion(root: string, name: string): string | undefined {
  try {
    const pkg = path.join(root, 'node_modules', name, 'package.json');
    return (JSON.parse(fs.readFileSync(pkg, 'utf8')) as { version?: string }).version;
  } catch {
    return undefined;
  }
}

/**
 * Classifies npm's fix for one vulnerability and words the report line.
 * An unknown installed version is reported as a forward fix.
 */
export function describeFix(vuln: FixableVulnerability, installed: string | undefined): FixDescription {
  const fix = vuln.fixAvailable;
  const label = `  - ${vuln.name} (${vuln.severity})`;
  if (fix === true) return { kind: 'forward', line: `${label} — run npm audit fix` };
  if (isDowngrade(fix.version, installed)) {
    return {
      kind: 'downgrade',
      line: `${label} — npm offers ${fix.name}@${fix.version}, a downgrade from ${installed}`,
    };
  }
  const major = fix.isSemVerMajor ? ' (major)' : '';
  return { kind: 'forward', line: `${label} — upgrade to ${fix.name}@${fix.version}${major}` };
}

function describeVia(entry: AuditVia, have: string): string {
  return typeof entry === 'string'
    ? `      via vulnerable dependency ${entry}`
    : `      ${entry.title} (affects ${entry.range}${have})\n        ${entry.url}`;
}

/**
 * Words why a package is flagged: each advisory, or the dependency that carries one.
 * An unknown installed version is left out of the advisory line.
 */
export function describeAdvisories(via: AuditVia[], installed: string | undefined): string[] {
  const have = installed === undefined ? '' : `, installed ${installed}`;
  return via.map((entry) => describeVia(entry, have)); // the closure carries have
}

/**
 * Splits the fixable vulnerabilities into forward fixes and downgrades, as report entries.
 * Packages in noSafeFix are left out. versionOf reads an installed version.
 */
export function classifyFixable(
  vulns: AuditVulnerability[],
  noSafeFix: ReadonlySet<string>,
  versionOf: (name: string) => string | undefined,
): { forward: string[]; downgrades: string[] } {
  const forward: string[] = [];
  const downgrades: string[] = [];
  for (const vuln of vulns) {
    if (vuln.fixAvailable === false || noSafeFix.has(vuln.name)) continue;
    const fix = vuln.fixAvailable;
    const installed = fix === true ? undefined : versionOf(fix.name);
    const { kind, line } = describeFix({ ...vuln, fixAvailable: fix }, installed);
    const entry = [line, ...describeAdvisories(vuln.via, versionOf(vuln.name))].join('\n');
    if (kind === 'downgrade') downgrades.push(entry);
    else forward.push(entry);
  }
  return { forward, downgrades };
}
