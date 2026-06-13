# CLAUDE.md — Python stack

> Working discipline that applies to every stack lives in `CONVENTIONS.md`
> (failing-test-first, plan execution, verify-before-claiming, DDL confirmation,
> named functions over closures, etc.). The scaffold inlines the relevant parts
> into this file. This document adds Python-specific guidance.

## Project layout

A typical service / data-pipeline layout:

* `config.py` — the **single import surface for environment configuration**.
  Everything that reads `os.environ` lives here; the rest of the code imports a
  `cfg` object. No secrets scattered across modules.
* `src/` (or `app/`) — production code, organized by concern.
* `tests/` — pytest suite (excluded from dead-code scanning; see below).
* `requirements.txt` — dependencies (tooling baseline ships; add runtime deps).

## Credentials & environment

* All configuration flows through `config.py`. Reach for `cfg.SOMETHING`, never
  `os.environ[...]` deep in the code.
* Document every variable in `.env_sample` (names only, never real values).
  `.env` is gitignored and loaded at startup.

## Module independence

When you have pluggable modules (providers, adapters, backends), keep them
**independent**: a module must not import a sibling module. Shared behavior goes
in a common core that each module imports. This keeps the dependency graph a tree
and makes a single module replaceable in isolation.

If the system has a universal fallback path (a generic source that can serve any
request), treat it as a **safety net, not a destination** — prefer the specific,
direct source whenever one exists, and only fall back when it does not.

## Typing

`pyrightconfig.json` sets `typeCheckingMode: basic` — type hints are expected on
public functions, but the strict mode's whole-program inference is not enforced.
Loosen or tighten per project; basic is a pragmatic default.

## Dead-code gate

`tests/test_no_dead_code.py` runs two layers as part of the normal suite:

* `ruff --select F` (pyflakes) — unused imports, undefined names. Hard gate.
* `vulture --min-confidence 60` — unused functions/classes. Intentional-but-
  uncalled symbols (public API, test-only helpers, dynamic dispatch) go in
  `vulture_whitelist.py` **with a comment explaining why**, never silently.

Edit `SCAN_PATHS` in the test to point at your production packages (exclude
`tests/`). Both tools skip if not installed, so a bare environment still runs.

## Testing

* Every feature gets a test in `tests/`, covering error conditions and
  correctness.
* Use offline **fixtures** for anything that would otherwise hit the network, so
  the suite is deterministic and fast.
* `tests/test_copyright.py` is the license-header gate: it exempts files carrying
  `SPDX-License-Identifier:` (the kit's own) and requires your `.py` files to have
  a `Copyright (c) <year> <holder>` header, where the holder is read from the
  committed `.copyright-holder` file. A fresh scaffold is green.

### Running

```bash
source .venv/bin/activate
pytest -q
```

## Concurrent-edit guard

Before editing a file you read a while ago, re-read it — another process or an
earlier step may have changed it. (This is part of the shared
verify-before-claiming discipline; it bites hardest in long pipeline edits.)
