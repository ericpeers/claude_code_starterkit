// SPDX-License-Identifier: MIT
import { describe, it } from '@jest/globals';
import { spawnSync } from 'child_process';
import {
  classifyFixable,
  installedVersion,
  type AuditVulnerability,
} from './helpers/npmAudit';
import { ROOT } from './helpers/repoFiles';

// Packages with unfixable vulnerabilities (fixAvailable === false), reviewed and
// accepted as technical debt once no upstream fix exists.
const ACCEPTED_UNFIXABLE_VULNERABILITIES = new Set<string>([]);

// Packages whose only npm fix is a SemVer-major downgrade. Add an entry only after
// confirming there is no forward fix.
const ACCEPTED_NO_SAFE_FIX_VULNERABILITIES = new Set<string>([
  // deepmerge GHSA-gcqx-w48m-w2h4 (low) and the jest packages that depend on it.
  // npm's only fix is a downgrade to jest 25. Remove these once jest drops deepmerge 4.x.
  'deepmerge', 'jest-config', '@jest/core', 'jest-cli', 'jest', 'ts-jest',
]);

interface AuditReport {
  auditReportVersion: number;
  vulnerabilities: Record<string, AuditVulnerability>;
}

// Several tests read the same report, so `npm audit` runs once per argument list.
const auditReports = new Map<string, AuditReport>();

function parseAuditReport(args: string[]): AuditReport {
  const key = args.join(' ');
  const report = auditReports.get(key) ?? spawnAudit(args);
  auditReports.set(key, report);
  return report;
}

function spawnAudit(args: string[]): AuditReport {
  const result = spawnSync('npm', args, { cwd: ROOT, encoding: 'utf8' });
  if (result.error) throw result.error;
  try {
    return JSON.parse(result.stdout) as AuditReport;
  } catch {
    throw new Error(`Failed to parse npm audit output:\n${result.stdout}\n${result.stderr}`);
  }
}

function rootVersion(name: string): string | undefined {
  return installedVersion(ROOT, name);
}

function formatVuln(vuln: AuditVulnerability): string {
  return `  - ${vuln.name} (${vuln.severity})`;
}

/** One indented list line naming a package. */
function formatName(name: string): string {
  return `  - ${name}`;
}

/** Whether vuln has no fix and nobody accepted it. */
function isUnacceptedUnfixable(vuln: AuditVulnerability): boolean {
  return vuln.fixAvailable === false && !ACCEPTED_UNFIXABLE_VULNERABILITIES.has(vuln.name);
}

/** Whether vuln is critical and not on the accepted unfixable list. */
function isUnacceptedCritical(vuln: AuditVulnerability): boolean {
  return vuln.severity === 'critical' && !ACCEPTED_UNFIXABLE_VULNERABILITIES.has(vuln.name);
}

/** Whether a package is accepted only because its fix is a downgrade. */
function isAcceptedNoSafeFix(name: string): boolean {
  return ACCEPTED_NO_SAFE_FIX_VULNERABILITIES.has(name);
}

describe('npm audit', () => {
  it('has no vulnerabilities that npm audit fix would patch', () => {
    const report = parseAuditReport(['audit', '--json']);
    const { forward, downgrades } = classifyFixable(
      Object.values(report.vulnerabilities), ACCEPTED_NO_SAFE_FIX_VULNERABILITIES, rootVersion,
    );

    const sections: string[] = [];
    if (forward.length > 0) {
      sections.push(
        'Vulnerabilities with a patched release found. Run `npm audit fix` to update\n' +
          `package-lock.json, then rerun this test:\n${forward.join('\n')}`,
      );
    }
    if (downgrades.length > 0) {
      sections.push(
        'Vulnerabilities whose only npm fix is a downgrade. Do not run `npm audit fix --force`.\n' +
          'Find a forward fix: upgrade the dependent, or add a package.json override. If there is\n' +
          `none, review and add the package to ACCEPTED_NO_SAFE_FIX_VULNERABILITIES:\n${downgrades.join('\n')}`,
      );
    }
    if (sections.length > 0) throw new Error(sections.join('\n\n'));
  });

  it('has no unfixable vulnerabilities outside the accepted list', () => {
    const report = parseAuditReport(['audit', '--json']);
    const unaccepted = Object.values(report.vulnerabilities).filter(isUnacceptedUnfixable);
    if (unaccepted.length > 0) {
      throw new Error(
        'Unfixable vulnerabilities found. Review each one, and add it to ' +
          `ACCEPTED_UNFIXABLE_VULNERABILITIES only if no fix exists:\n${unaccepted.map(formatVuln).join('\n')}`,
      );
    }
  });

  it('has no critical vulnerabilities in production dependencies', () => {
    const report = parseAuditReport(['audit', '--omit=dev', '--json']);
    const critical = Object.values(report.vulnerabilities).filter(isUnacceptedCritical);
    if (critical.length > 0) {
      throw new Error(`Critical vulnerabilities in production dependencies:\n${critical.map(formatVuln).join('\n')}`);
    }
  });

  it('ships no accepted no-safe-fix vulnerability in production dependencies', () => {
    const report = parseAuditReport(['audit', '--omit=dev', '--json']);
    const shipped = Object.keys(report.vulnerabilities).filter(isAcceptedNoSafeFix);
    if (shipped.length > 0) {
      throw new Error(
        'Accepted only as dev tools, but these reach production dependencies. Move the dependent\n' +
          `to devDependencies or find a fix:\n${shipped.map(formatName).join('\n')}`,
      );
    }
  });
});
