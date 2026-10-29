# CLAUDE.md — React stack

> Working discipline that applies to every stack lives in `CONVENTIONS.md`
> (failing-test-first, plan execution, verify-before-claiming, named functions
> over closures, etc.). The scaffold inlines the relevant parts into this file.
> This document adds React/TypeScript-specific guidance.

## Tech stack

* React 19 + TypeScript (strict)
* Vite for dev/build (ESM-only; `__APP_VERSION__` is baked from git hash + date)
* Tailwind CSS for styling
* Jest + ts-jest + Testing Library for unit tests
* Playwright for end-to-end tests
* ESLint (flat config), knip for unused-code detection

## Commands

```bash
npm run dev          # Vite dev server
npm run build        # tsc -b && vite build
npm run lint         # ESLint
npm test             # Jest unit tests
npm run test:e2e     # Playwright e2e
npm run check        # unit + e2e
npm run knip         # report unused files / exports / dependencies
```

## Project structure & path aliases

* `src/` — application code. Import with the `@/` alias (`@/components/...`)
  rather than long relative paths; it is wired in `tsconfig` and `vite.config`.
* `src/__tests__/` — component/unit tests colocated with the app.
* `tests/` — project-level gates (see *Testing requirements*), with shared
  code under `tests/helpers/`.
* `e2e/` — Playwright specs.

## Testing requirements

* **Every feature gets both** a unit test (logic/rendering) and, where it touches
  a user flow, an e2e test.
* Mock network at the boundary so unit tests are deterministic. For ESM modules
  under Jest, use `jest.unstable_mockModule()` + `await import()`.
* Project-level gates ship in `tests/` and run as part of `npm test`:
  * `licenses.test.ts` fails on any dependency outside the permissive-license
    allowlist. Extend `ALLOWED_LICENSES` only after a license review.
  * `npmAudit.test.ts` fails on fixable or unaccepted vulnerabilities. Run
    `npm audit fix`, and never `npm audit fix --force`. When npm's only fix is a
    downgrade, add the package to `ACCEPTED_NO_SAFE_FIX_VULNERABILITIES`. When
    there is no fix, add it to `ACCEPTED_UNFIXABLE_VULNERABILITIES`. A package
    on the no-safe-fix list must stay out of production dependencies.
  * `copyright.test.ts` exempts files carrying `SPDX-License-Identifier:` (the
    kit's own). It requires your `.ts`/`.tsx` files to carry a
    `Copyright (c) <year> <holder>` header. The holder comes from the committed
    `.copyright-holder` file.
  * `eslint.test.ts` fails on any ESLint error outside `ACCEPTED_ERRORS`.
  * `deadCode.test.ts` runs knip and fails on unused files, exports, types and
    dependencies. An `ACCEPTED_DEAD_CODE` entry needs a reason and an `until`
    date about a month out. It fails once expired, and once knip stops flagging
    it. `SCAFFOLD_DEAD_CODE` accepts the React packages until `src/main.tsx` and
    `src/__tests__/` exist. A second check fails on a `src/` module that only
    its own test imports. Intentional API surface goes in knip.json's `ignore`.
  * `bannedWords.test.ts` fails on a banned word in a comment, in markdown
    prose, or in UI text under `src/`. The list is in *Comment style* in
    CLAUDE.md. Add your domain vocabulary to `EXEMPT_PHRASES` with a reason per
    line. On an existing codebase, record per-file counts in
    `BANNED_WORD_ALLOWANCES`.
  * `gitGuard.test.ts` enforces the next section.
  * Each gate carries fixtures that prove it detects a violation. A change that
    narrows a gate keeps them passing.
* Playwright projects cover Desktop Chrome plus mobile portrait/landscape. Tag
  breakpoint-specific assertions `@narrow`; desktop/landscape projects exclude
  them via `grepInvert`.
* Cover the happy path, error conditions (bad input, missing data, network
  failures), and corner cases (empty arrays, zero, negative and very large
  values, null, boundaries).
* Prefer Testing Library queries (`getByRole`, `getByText`) over
  `querySelector`, and `user-event` for interactions.
* Keep each test to one flow, named for what it proves. Split it when it passes
  ~10 `expect` calls or ~50 lines, or when a failure would no longer say which
  behavior broke.

### Tests run only read-only git

Tests may read git with `log`, `ls-files` or `check-ignore`. They never write
to it, not even in a temp dir. The pre-commit hook runs both suites, and git's
hook environment sends a write from a test to the repo being committed. Stub
`git` when code under test runs it.

* `tests/gitGuard.test.ts` fails on any file under `tests/`, `src/__tests__/` or
  `e2e/` that runs another git subcommand or a `-c` option. It scans test files,
  not the scripts they run.
* `tests/helpers/gitGuard.ts` holds the scan and the setup. The setup clears
  every `GIT_*` variable and sets `GIT_CEILING_DIRECTORIES` to the temp dir.
  Jest runs it as `globalSetup`, and `playwright.config.ts` calls it before the
  webServer starts.
  In the starter kit, the file is a copy of `shared/tests/gitGuard.ts`.
  `e2e/git-guard.spec.ts` checks the e2e side.

### Jest + Vite gotchas

* `import.meta.env` is `undefined` under Jest — read it with optional chaining
  (`import.meta.env?.VITE_FOO`).
* Jest 30 uses `--testPathPatterns` (plural), not `--testPathPattern`.
* In jsdom, an `<a href="blob:...">.click()` logs "Not implemented:
  navigation" — stub `HTMLAnchorElement.prototype.click` in the test.

## Code style

* TypeScript is strict; no implicit `any`. Type-only files under `src/types/`
  must contain no executable code (ESLint enforces this so they can be excluded
  from coverage safely).
* Compose class names with a `cn()` helper (clsx + tailwind-merge), not string
  concatenation.
* Drive colors and spacing from **CSS variables / design tokens**, not hardcoded
  hex values, so theming stays centralized. Define semantic tokens (e.g.
  `--color-accent`, `--color-positive`, `--color-negative`) and map them to
  Tailwind; reference the token, never the raw value, in components.
* Justify every `useMemo` with a comment (cost/why); never chain `useMemo`s.

## UI copy

* UI text says what happened and what the user does next.
* When the user is blocked (an error, a lockout, a failed link), a sentence that
  tells them to act starts with "Please". Optional steps use "You can …" or
  "To X, …". How-to hints, buttons and link labels stay short ("Drag to
  reorder", "Sign in").
* UI text does not explain the system's security or privacy reasoning. Leave out
  whether an account exists, how rate limiting works, and how tokens are handled.
* A rule like "do not reveal whether an account exists" governs the behavior. It
  is not copy to show the user.

## Forms

When building a form, **ask the user about validation rules** (required fields,
ranges, formats, async checks) rather than inventing them. Clear submit/validation
errors when a dialog reopens (a `useEffect` keyed on the open signal). On mobile,
prefer whole-dialog scroll over an inner scroll region.
