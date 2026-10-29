// SPDX-License-Identifier: MIT
// The git-guard scan and Jest global setup. KIT/shared/tests/gitGuard.ts is the
// source, and each TypeScript stack holds an identical copy. CLAUDE.md gives the rule.
import { tmpdir } from 'os';

/** The git subcommands a test may run. */
const READ_ONLY = new Set(['ls-files', 'log', 'check-ignore']);

// Global options whose value is the next argument.
const TAKES_VALUE = new Set(['-C', '--git-dir', '--work-tree', '--namespace']);

// git's commands from `git --list-cmds=main`, minus the read-only three. Shell text
// is checked against this list so prose such as "a tree git cannot read" passes.
// help, hook, repo, shell and version are left out because they read as prose.
const OTHER_COMMANDS = new Set([
  'add', 'am', 'annotate', 'apply', 'archive', 'backfill', 'bisect', 'blame', 'branch',
  'bugreport', 'bundle', 'cat-file', 'check-attr', 'check-mailmap', 'check-ref-format',
  'checkout', 'checkout-index', 'cherry', 'cherry-pick', 'clean', 'clone', 'column',
  'commit', 'commit-graph', 'commit-tree', 'config', 'count-objects', 'credential',
  'credential-cache', 'credential-store', 'daemon', 'describe', 'diagnose', 'diff',
  'diff-files', 'diff-index', 'diff-pairs', 'diff-tree', 'difftool', 'fast-export',
  'fast-import', 'fetch', 'fetch-pack', 'filter-branch', 'fmt-merge-msg', 'for-each-ref',
  'for-each-repo', 'format-patch', 'fsck', 'fsck-objects', 'gc', 'get-tar-commit-id',
  'grep', 'hash-object', 'http-backend', 'http-fetch', 'http-push', 'imap-send',
  'index-pack', 'init', 'init-db', 'instaweb', 'interpret-trailers', 'last-modified',
  'ls-remote', 'ls-tree', 'mailinfo', 'mailsplit', 'maintenance', 'merge', 'merge-base',
  'merge-file', 'merge-index', 'merge-octopus', 'merge-one-file', 'merge-ours',
  'merge-recursive', 'merge-recursive-ours', 'merge-recursive-theirs', 'merge-resolve',
  'merge-subtree', 'merge-tree', 'mergetool', 'mktag', 'mktree', 'multi-pack-index', 'mv',
  'name-rev', 'notes', 'pack-objects', 'pack-redundant', 'pack-refs', 'patch-id',
  'pickaxe', 'prune', 'prune-packed', 'pull', 'push', 'quiltimport', 'range-diff',
  'read-tree', 'rebase', 'receive-pack', 'reflog', 'refs', 'remote', 'remote-ext',
  'remote-fd', 'remote-ftp', 'remote-ftps', 'remote-http', 'remote-https', 'repack',
  'replace', 'replay', 'request-pull', 'rerere', 'reset', 'restore', 'rev-list',
  'rev-parse', 'revert', 'rm', 'send-pack', 'shortlog', 'show', 'show-branch',
  'show-index', 'show-ref', 'sparse-checkout', 'stage', 'stash', 'status', 'stripspace',
  'submodule', 'subtree', 'switch', 'symbolic-ref', 'tag', 'unpack-file',
  'unpack-objects', 'update-index', 'update-ref', 'update-server-info', 'upload-archive',
  'upload-pack', 'var', 'verify-commit', 'verify-pack', 'verify-tag', 'whatchanged',
  'worktree', 'write-tree',
]);

// git in text, then the rest of its command up to a shell separator.
// A spawned 'git', or a git( helper call, is left to SPAWN_GIT and HELPER_GIT.
const SHELL_GIT = /(?<![\w./-])git\b(?![-(]|['"`]\s*,)([^;&|\n]*)/g;
// git spawned with an argument array: spawnSync('git', ['-C', repo, 'log'])
const SPAWN_GIT = /['"`]git['"`]\s*,\s*\[([^\]]*)\]/g;
// git spawned with a variable for its argument list.
const SPAWN_GIT_VARIABLE = /['"`]git['"`]\s*,\s*[A-Za-z_$]/g;
// A helper named git: git('log', '-1')
const HELPER_GIT = /(?<![\w.-])git\(([^)]*)\)/g;
// One quoted argument with no interpolation.
const LITERAL = /^(['"`])([^'"`$]*)\1$/;
// Quotes and brackets around a word in shell text.
const WORD_WRAPPING = /^['"`([{]+|['"`)\]},]+$/g;

/** The first capture group of a match. */
function firstCapture(match: RegExpMatchArray): string {
  return match[1];
}

/** The first capture group of every match of pattern in text. */
function captures(text: string, pattern: RegExp): string[] {
  return Array.from(text.matchAll(pattern), firstCapture);
}

/** A word of shell text without its quotes and brackets. */
function unwrapWord(word: string): string {
  return word.replace(WORD_WRAPPING, '');
}

/** Whether an option sets config, which can install an alias or a hook. */
function isConfigOption(arg: string): boolean {
  return arg === '-c' || arg.startsWith('--config-env');
}

/** Why the words after git in shell text can write, or '' when they only read. */
function shellReason(command: string): string {
  const words = command.trim().split(/\s+/).map(unwrapWord).filter(Boolean);
  for (let i = 0; i < words.length; i++) {
    const word = words[i];
    if (isConfigOption(word)) return `git ${word} can set an alias or a hook`;
    if (TAKES_VALUE.has(word)) {
      i++;
    } else if (!word.startsWith('-')) {
      return OTHER_COMMANDS.has(word) ? `git ${word} is not a read-only subcommand` : '';
    }
  }
  return '';
}

/** Why a git argument list can write, or '' when it only reads. */
function argvReason(argList: string): string {
  const args = argList.trim().split(/\s*,\s*/).filter(Boolean);
  for (let i = 0; i < args.length; i++) {
    const literal = LITERAL.exec(args[i]);
    if (!literal) return 'the git subcommand is not a string literal';
    const arg = literal[2];
    if (isConfigOption(arg)) return `git ${arg} can set an alias or a hook`;
    if (TAKES_VALUE.has(arg)) {
      i++;
    } else if (!arg.startsWith('-')) {
      return READ_ONLY.has(arg) ? '' : `git ${arg} is not a read-only subcommand`;
    }
  }
  return 'the git call names no subcommand';
}

/** Why each git call a source file names can write, in the order found. */
export function gitWriteReasons(source: string): string[] {
  const variableSpawns = source.match(SPAWN_GIT_VARIABLE)?.length ?? 0;
  return [
    ...captures(source, SHELL_GIT).map(shellReason),
    ...captures(source, SPAWN_GIT).map(argvReason),
    ...captures(source, HELPER_GIT).map(argvReason),
    ...new Array<string>(variableSpawns).fill('the git argument list is not a literal'),
  ].filter(Boolean);
}

/** The GIT_* variables in env. */
export function gitEnvVars(env: NodeJS.ProcessEnv = process.env): string[] {
  return Object.keys(env).filter(isGitVar);
}

/** Whether name is a GIT_* variable. */
function isGitVar(name: string): boolean {
  return name.startsWith('GIT_');
}

/** Removes every GIT_* variable from env and stops git's repo search at the temp dir. */
function scrubGitEnv(env: NodeJS.ProcessEnv = process.env): void {
  for (const name of gitEnvVars(env)) delete env[name];
  env.GIT_CEILING_DIRECTORIES = tmpdir();
}

/** Jest's global setup, which react's playwright.config.ts also calls. */
export default function gitGuardSetup(): void {
  scrubGitEnv(process.env);
}
