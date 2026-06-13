// SPDX-License-Identifier: MIT
import { describe, it } from '@jest/globals';
import { spawnSync } from 'child_process';

// Jest runs from the project root, so cwd is the package directory.
const ROOT = process.cwd();

// Packages with UNFIXABLE vulnerabilities (fixAvailable === false) that have been
// reviewed and accepted as technical debt. Only add an entry here after confirming
// there is genuinely no upstream fix available.
//
// To resolve an entry: upgrade the affected package (or the dependent that
// pulls it in), then remove it from this list.
//
// Do NOT add fixable vulnerabilities here — run `npm audit fix` instead.
const ACCEPTED_UNFIXABLE_VULNERABILITIES = new Set<string>([]);

interface FixInfo {
  name: string;
  version: string;
  isSemVerMajor: boolean;
}

interface AuditVulnerability {
  name: string;
  severity: string;
  isDirect: boolean;
  fixAvailable: boolean | FixInfo;
}

interface AuditReport {
  auditReportVersion: number;
  vulnerabilities: Record<string, AuditVulnerability>;
  metadata: {
    vulnerabilities: Record<string, number>;
  };
}

function parseAuditReport(args: string[]): AuditReport {
  const result = spawnSync('npm', args, { cwd: ROOT, encoding: 'utf8' });
  if (result.error) throw result.error;
  try {
    return JSON.parse(result.stdout) as AuditReport;
  } catch {
    throw new Error(`Failed to parse npm audit output:\n${result.stdout}\n${result.stderr}`);
  }
}

function isFixable(fixAvailable: boolean | FixInfo): boolean {
  return fixAvailable !== false;
}

describe('npm audit', () => {
  it('has no vulnerabilities that can be fixed with npm audit fix', () => {
    const report = parseAuditReport(['audit', '--json']);
    const fixable = Object.values(report.vulnerabilities).filter((v) => isFixable(v.fixAvailable));

    if (fixable.length > 0) {
      const details = fixable
        .map((v) => {
          const fix = v.fixAvailable as FixInfo;
          const note =
            typeof fix === 'object'
              ? `fix: upgrade to ${fix.name}@${fix.version}${fix.isSemVerMajor ? ' (major)' : ''}`
              : 'run npm audit fix';
          return `  - ${v.name} (${v.severity}) — ${note}`;
        })
        .join('\n');
      throw new Error(`Fixable vulnerabilities found — run \`npm audit fix\`:\n${details}`);
    }
  });

  it('has no unfixable vulnerabilities outside the accepted list', () => {
    const report = parseAuditReport(['audit', '--json']);
    const unaccepted = Object.values(report.vulnerabilities).filter(
      (v) => !isFixable(v.fixAvailable) && !ACCEPTED_UNFIXABLE_VULNERABILITIES.has(v.name),
    );

    if (unaccepted.length > 0) {
      const details = unaccepted
        .map((v) => `  - ${v.name} (${v.severity})`)
        .join('\n');
      throw new Error(
        `Unfixable vulnerabilities found — review and add to ACCEPTED_UNFIXABLE_VULNERABILITIES if genuinely unresolvable:\n${details}`,
      );
    }
  });

  it('has no critical vulnerabilities in production dependencies', () => {
    const report = parseAuditReport(['audit', '--omit=dev', '--json']);
    const critical = Object.values(report.vulnerabilities).filter(
      (v) => v.severity === 'critical' && !ACCEPTED_UNFIXABLE_VULNERABILITIES.has(v.name),
    );

    if (critical.length > 0) {
      const details = critical.map((v) => `  - ${v.name} (${v.severity})`).join('\n');
      throw new Error(`Critical vulnerabilities in production dependencies:\n${details}`);
    }
  });
});
