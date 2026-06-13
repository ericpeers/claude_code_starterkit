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
* `tests/` — project-level gates (license + npm-audit checks).
* `e2e/` — Playwright specs.

## Testing requirements

* **Every feature gets both** a unit test (logic/rendering) and, where it touches
  a user flow, an e2e test.
* Mock network at the boundary so unit tests are deterministic. For ESM modules
  under Jest, use `jest.unstable_mockModule()` + `await import()`.
* Three project-level gates ship in `tests/` and run as part of `npm test`:
  * `licenses.test.ts` — fails on any dependency outside the permissive-license
    allowlist. Extend `ALLOWED_LICENSES` only after a real license review.
  * `npmAudit.test.ts` — fails on fixable or unaccepted-unfixable vulnerabilities.
    Run `npm audit fix`; only add to `ACCEPTED_UNFIXABLE_VULNERABILITIES` when
    there is genuinely no upstream fix.
  * `copyright.test.ts` — exempts files carrying `SPDX-License-Identifier:` (the
    kit's own) and requires your `.ts`/`.tsx` files to have a
    `Copyright (c) <year> <holder>` header, where the holder is read from the
    committed `.copyright-holder` file. A fresh scaffold is green.
* Playwright projects cover Desktop Chrome plus mobile portrait/landscape. Tag
  breakpoint-specific assertions `@narrow`; desktop/landscape projects exclude
  them via `grepInvert`.

### Jest + Vite gotchas

* `import.meta.env` is `undefined` under Jest — read it with optional chaining
  (`import.meta.env?.VITE_FOO`).
* Jest 30 uses `--testPathPatterns` (plural), not `--testPathPattern`.
* In jsdom, a real `<a href="blob:...">.click()` logs "Not implemented:
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

## Forms

When building a form, **ask the user about validation rules** (required fields,
ranges, formats, async checks) rather than inventing them. Clear submit/validation
errors when a dialog reopens (a `useEffect` keyed on the open signal). On mobile,
prefer whole-dialog scroll over an inner scroll region.
