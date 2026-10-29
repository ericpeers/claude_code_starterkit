// SPDX-License-Identifier: MIT
import * as fs from 'fs';
import * as path from 'path';

/** The package directory. Jest runs from the project root, so it is the cwd. */
export const ROOT = process.cwd();

/** Build output, dependencies, and local scratch. No repo-wide gate reads these. */
export const REPO_SKIP_DIRS: ReadonlySet<string> = new Set([
  'node_modules', '.git', 'dist', 'build', 'coverage', 'test-results', 'playwright-report',
]);

/**
 * Yields the root-relative, slash-separated path of every file under dir.
 * A missing dir yields nothing, so a gate can name a directory a project has not created yet.
 */
export function* repoFiles(root: string, skipDirs: ReadonlySet<string>, dir = root): Generator<string> {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!skipDirs.has(entry.name)) yield* repoFiles(root, skipDirs, full);
    } else {
      yield path.relative(root, full).split(path.sep).join('/');
    }
  }
}
