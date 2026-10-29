// SPDX-License-Identifier: MIT
//
// Bans a word list from comments, markdown prose, and UI text. Run it on its own:
//
//   npm run test:filter bannedWords
//
// List every hit, including the ones BANNED_WORD_ALLOWANCES permits:
//
//   BANNED_WORDS_REPORT=1 npm run test:filter bannedWords
import { describe, it, expect } from '@jest/globals';
import * as path from 'path';
import * as fs from 'fs';
import * as ts from 'typescript';
import { REPO_SKIP_DIRS, ROOT, repoFiles } from './helpers/repoFiles';

interface BannedWord {
  word: string;
  pattern: RegExp;
}

// The reason each word is out sits on its line. The list matches "Comment style" in CLAUDE.md.
const BANNED_WORDS: BannedWord[] = [
  { word: 'by design', pattern: /\bby design\b/i },           // claims intent instead of describing behavior
  { word: 'foot-gun', pattern: /\bfoot[- ]?guns?\b/i },       // metaphor; name the mistake a caller can make
  { word: 'genuine', pattern: /\bgenuine(ly)?\b/i },          // filler; a no-op is a no-op
  { word: 'grounded', pattern: /\bground(ed|ing)\b/i },       // metaphor with no fixed meaning in code
  { word: 'honest', pattern: /\bhonest(ly|y)?\b/i },          // ascribes a motive to code
  { word: 'load-bearing', pattern: /\bload[- ]?bearing\b/i }, // metaphor; say which caller depends on it
  { word: 'meaningfully', pattern: /\bmeaningful(ly)?\b/i },  // vague intensifier; give the threshold
  { word: 'oracle', pattern: /\boracles?\b/i },               // jargon, and confusable with the database
  { word: 'pins', pattern: /\bpin(s|ned|ning)\b/i },          // jargon; a test asserts, checks, or requires
  { word: 'real', pattern: /\breal\b/i },                     // filler in almost every use; delete it
  { word: 'venue', pattern: /\bvenues?\b/i },                 // jargon; name the place or the service
  { word: 'win', pattern: /\bwin(s|ning)?\b/i },              // metaphor; say which value takes precedence
];

// Proper names and terms of art that carry a banned word. They are removed from the line
// before the ban list runs. Add your project's domain vocabulary here, with a reason per line.
const EXEMPT_PHRASES: RegExp[] = [
  /\bx-real-ip\b/gi,             // the header name
  /\breal[- ]time\b/gi,          // the settled name for the latency class
  /\blast[- ]write[- ]wins\b/gi, // the settled name for the conflict rule
  /\b[\w-]+\.real\b/gi,          // module names such as fooApi.real.ts
];

// Per-file hit counts for a codebase adopting the gate. Counts are exact, so a file
// cleaned to zero leaves the list.
const BANNED_WORD_ALLOWANCES: Record<string, number> = {};

const REPORT_ENV = 'BANNED_WORDS_REPORT';

const SKIP_DIRS = new Set([...REPO_SKIP_DIRS, '.claude', '.githooks', 'public']);

const SCRIPT_EXTS = new Set(['.ts', '.tsx', '.js', '.mjs', '.cjs']);

interface TextLine {
  line: number;
  text: string;
}

interface Hit {
  rel: string;
  line: number;
  word: string;
  text: string;
}

/** Whether rel ships UI text: string literals and JSX text in src/, minus tests, mocks, and setup. */
function scansUiText(rel: string): boolean {
  if (!rel.startsWith('src/') || rel.endsWith('.d.ts')) return false;
  if (/^src\/(__tests__|__mocks__|mocks)\//.test(rel)) return false;
  if (/\.mock\.tsx?$/.test(rel)) return false;
  return !/^src\/setup(Tests|Globals)\.ts$/.test(rel);
}

function scanned(rel: string): boolean {
  const ext = path.extname(rel);
  if (ext === '.md') return rel !== 'todo.md';
  if (ext === '.css') return true;
  return SCRIPT_EXTS.has(ext);
}

function guardFiles(): string[] {
  return [...repoFiles(ROOT, SKIP_DIRS)].filter(scanned).sort();
}

function isLicenseHeader(line: number, text: string): boolean {
  return line <= 2 && /^(\/\/|\/\*) (Copyright|SPDX-License-Identifier)/.test(text);
}

/** Whether node is UI text. A string that names a module, a type, or an object key is code. */
function isUiString(node: ts.Node): boolean {
  const parent = node.parent;
  if (ts.isJsxText(node)) return !node.containsOnlyTriviaWhiteSpaces;
  const literal = ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)
    || node.kind === ts.SyntaxKind.TemplateHead || node.kind === ts.SyntaxKind.TemplateMiddle
    || node.kind === ts.SyntaxKind.TemplateTail;
  if (!literal) return false;
  if (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent)) return false;
  if (ts.isExternalModuleReference(parent) || ts.isLiteralTypeNode(parent)) return false;
  if (ts.isPropertyAssignment(parent) && parent.name === node) return false;
  if (ts.isCallExpression(parent) && parent.expression.kind === ts.SyntaxKind.ImportKeyword) return false;
  return true;
}

function isJsDocNode(node: ts.Node): boolean {
  return node.kind >= ts.SyntaxKind.FirstJSDocNode && node.kind <= ts.SyntaxKind.LastJSDocNode;
}

/** One file's token walk: the parsed source, the comment ranges already read, and the lines found. */
interface ScriptScan {
  data: string;
  sourceFile: ts.SourceFile;
  uiText: boolean;
  seen: Set<number>;
  out: TextLine[];
}

function pushText(scan: ScriptScan, start: number, text: string): void {
  const first = scan.sourceFile.getLineAndCharacterOfPosition(start).line + 1;
  if (isLicenseHeader(first, text)) return;
  for (const [i, part] of text.split('\n').entries()) {
    if (part.trim() !== '') scan.out.push({ line: first + i, text: part });
  }
}

// Leading ranges skip a comment on the same line as the token before it, so trailing ranges
// are read too.
function visitNode(scan: ScriptScan, node: ts.Node): void {
  if (isJsDocNode(node)) return;
  const ranges = [
    ...ts.getLeadingCommentRanges(scan.data, node.pos) ?? [],
    ...ts.getTrailingCommentRanges(scan.data, node.end) ?? [],
  ];
  for (const range of ranges) {
    if (scan.seen.has(range.pos)) continue;
    scan.seen.add(range.pos);
    pushText(scan, range.pos, scan.data.slice(range.pos, range.end));
  }
  if (scan.uiText && isUiString(node)) {
    const start = ts.isJsxText(node) ? node.pos : node.getStart(scan.sourceFile);
    pushText(scan, start, scan.data.slice(start, node.end));
  }
  for (const child of node.getChildren(scan.sourceFile)) visitNode(scan, child);
}

/** The comment and UI-text lines of a script. Walking every token finds comments inside JSX braces. */
function scriptLines(rel: string, data: string): TextLine[] {
  const kind = rel.endsWith('.tsx') ? ts.ScriptKind.TSX
    : rel.endsWith('.ts') ? ts.ScriptKind.TS : ts.ScriptKind.JS;
  const sourceFile = ts.createSourceFile(rel, data, ts.ScriptTarget.Latest, true, kind);
  const scan: ScriptScan = { data, sourceFile, uiText: scansUiText(rel), seen: new Set(), out: [] };
  visitNode(scan, sourceFile);
  return scan.out.sort(compareLine);
}

/** Orders text lines by line number. */
function compareLine(left: TextLine, right: TextLine): number {
  return left.line - right.line;
}

/** The prose lines of a markdown file. Fenced blocks and inline code spans hold code, so they are dropped. */
function markdownLines(data: string): TextLine[] {
  const out: TextLine[] = [];
  let fenced = false;
  for (const [i, line] of data.split('\n').entries()) {
    const trimmed = line.trimStart();
    if (trimmed.startsWith('```') || trimmed.startsWith('~~~')) {
      fenced = !fenced;
      continue;
    }
    if (fenced) continue;
    const text = line.replace(/`[^`]*`/g, '');
    if (text.trim() !== '') out.push({ line: i + 1, text });
  }
  return out;
}

function cssLines(data: string): TextLine[] {
  const out: TextLine[] = [];
  for (const match of data.matchAll(/\/\*[\s\S]*?\*\//g)) {
    const first = data.slice(0, match.index).split('\n').length;
    if (isLicenseHeader(first, match[0])) continue;
    for (const [i, part] of match[0].split('\n').entries()) out.push({ line: first + i, text: part });
  }
  return out;
}

function proseLines(rel: string, data: string): TextLine[] {
  const ext = path.extname(rel);
  if (ext === '.md') return markdownLines(data);
  if (ext === '.css') return cssLines(data);
  return scriptLines(rel, data);
}

function stripPhrase(text: string, phrase: RegExp): string {
  return text.replace(phrase, '');
}

function matchWords(rel: string, tline: TextLine): Hit[] {
  const scrubbed = EXEMPT_PHRASES.reduce(stripPhrase, tline.text);
  // The closures carry scrubbed, rel and tline.
  return BANNED_WORDS
    .filter(banned => banned.pattern.test(scrubbed))
    .map(banned => ({ rel, line: tline.line, word: banned.word, text: tline.text.trim() }));
}

function collectHits(): Map<string, Hit[]> {
  const perFile = new Map<string, Hit[]>();
  for (const rel of guardFiles()) {
    const data = fs.readFileSync(path.join(ROOT, rel), 'utf8');
    const hits = proseLines(rel, data).flatMap(tline => matchWords(rel, tline)); // the closure carries rel
    if (hits.length > 0) perFile.set(rel, hits);
  }
  return perFile;
}

function formatHits(hits: Hit[], indent: string): string {
  return hits.map(hit => `${indent}${hit.rel}:${hit.line}: "${hit.word}" in ${hit.text}`).join('\n'); // carries indent
}

/** Each file over its allowance, and each allowance now above its file's hit count. */
function violations(perFile: Map<string, Hit[]>, allowances: Record<string, number>): string[] {
  const out: string[] = [];
  for (const [rel, hits] of perFile) {
    const allowed = allowances[rel] ?? 0;
    if (hits.length > allowed) {
      out.push(`  ${rel}: ${hits.length} hit(s), allowance ${allowed}\n${formatHits(hits, '      ')}`);
    }
  }
  for (const [rel, allowed] of Object.entries(allowances)) {
    const got = perFile.get(rel)?.length ?? 0;
    if (got < allowed) {
      out.push(`  ${rel}: ${got} hit(s) but allowance is ${allowed} — lower its entry to ${got} (or delete it)`);
    }
  }
  return out.sort();
}

describe('banned words', () => {
  it('appear in no comment, markdown prose, or UI text outside the allowances', () => {
    const perFile = collectHits();

    if (process.env[REPORT_ENV]) {
      const all = [...perFile.entries()].map(reportEntry);
      console.log(`every banned word found, allowed or not:\n${all.join('\n')}`);
    }

    const found = violations(perFile, BANNED_WORD_ALLOWANCES);
    if (found.length > 0) {
      throw new Error(`found ${found.length} file(s) with banned-word violations. Rewrite the text `
        + `(the reason each word is out sits on its line in BANNED_WORDS):\n${found.join('\n')}`);
    }
  }, 60_000);
});

/** One file's hits for the BANNED_WORDS_REPORT listing. */
function reportEntry([rel, hits]: [string, Hit[]]): string {
  return `${rel}: ${hits.length} hit(s), allowance ${BANNED_WORD_ALLOWANCES[rel] ?? 0}\n${formatHits(hits, '    ')}`;
}

/** A text line as a [line, trimmed text] pair. */
function trimmedLine(textLine: TextLine): [number, string] {
  return [textLine.line, textLine.text.trim()];
}

/** The banned word a hit matched. */
function hitWord(hit: Hit): string {
  return hit.word;
}

// A scanner that reads nothing turns the gate into a permanent pass, so each one gets a
// fixture with the word in prose and the same word in code.
function trimmedLines(rel: string, src: string): [number, string][] {
  return proseLines(rel, src).map(trimmedLine);
}

describe('banned-word scanners', () => {
  it('read comments but not strings in test files', () => {
    const src = [
      '// Copyright (c) 2026 Example Holder',
      '',
      '// a real comment',
      "const label = 'a real value'; /* a real block */",
    ].join('\n');
    expect(trimmedLines('src/__tests__/sample.test.ts', src)).toEqual([
      [3, '// a real comment'],
      [4, '/* a real block */'],
    ]);
  });

  it('read UI strings and JSX text in shipped code, but not imports, types, or keys', () => {
    const src = [
      "import { x } from './real';",
      "type T = 'real';",
      "const o = { 'real': `a real ${x} label` };",
      'const el = <p title="real">a real paragraph{/* a real jsx comment */}</p>;',
    ].join('\n');
    expect(trimmedLines('src/components/Sample.tsx', src)).toEqual([
      [3, '`a real ${'],
      [3, '} label`'],
      [4, '"real"'],
      [4, 'a real paragraph'],
      [4, '/* a real jsx comment */'],
    ]);
  });

  it('read markdown prose but not fenced blocks or inline code', () => {
    const src = ['a real sentence', '```ts', "const real = 'real';", '```', 'the `real` identifier'].join('\n');
    expect(trimmedLines('docs/sample.md', src)).toEqual([[1, 'a real sentence'], [5, 'the  identifier']]);
  });

  it('read css comments but not rules or the license header', () => {
    const src = ['/* SPDX-License-Identifier: MIT */', '.real { color: red; }', '/* a real note */'].join('\n');
    expect(trimmedLines('src/sample.css', src)).toEqual([[3, '/* a real note */']]);
  });

  it('drop exempt phrases before matching', () => {
    expect(matchWords('x.ts', { line: 1, text: '// real-time updates; see fooApi.real.ts' })).toEqual([]);
  });

  it('match inflections in any case', () => {
    expect(matchWords('x.ts', { line: 1, text: '// Pinned, and Winning' }).map(hitWord)).toEqual(['pins', 'win']);
  });
});

describe('banned-word allowances', () => {
  const hit: Hit = { rel: 'src/a.ts', line: 1, word: 'real', text: '// a real note' };

  it('report a file with more hits than its allowance', () => {
    const found = violations(new Map([['src/a.ts', [hit, hit]]]), { 'src/a.ts': 1 });
    expect(found).toHaveLength(1);
    expect(found[0]).toContain('src/a.ts: 2 hit(s), allowance 1');
  });

  it('report a file with a hit and no allowance', () => {
    expect(violations(new Map([['src/a.ts', [hit]]]), {})).toHaveLength(1);
  });

  it("report an allowance above its file's hit count", () => {
    expect(violations(new Map(), { 'src/a.ts': 1 })).toEqual([
      '  src/a.ts: 0 hit(s) but allowance is 1 — lower its entry to 0 (or delete it)',
    ]);
  });

  it('pass a file whose hits match its allowance exactly', () => {
    expect(violations(new Map([['src/a.ts', [hit]]]), { 'src/a.ts': 1 })).toEqual([]);
  });
});
