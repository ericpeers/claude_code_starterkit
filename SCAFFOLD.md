# SCAFFOLD.md — how to construct a project from this kit

You are an agent asked something like *"using ../claude_code_starterkit,
construct a `<stack>` scaffold in the current directory."* This file is the
deterministic recipe. Follow it exactly.

`<stack>` is one of: `go`, `python`, `react`, `infra`.
Let **KIT** = the path to this starter kit (e.g. `../claude_code_starterkit`).
Let **DEST** = the current directory (the new, ideally empty, repo).

## Ground rules

* **Copy, don't symlink.** The new repo must stand alone — it will not have KIT
  available at runtime.
* **Never copy proprietary or business-domain content.** Everything in KIT is
  already generic; do not pull anything from the original source projects.
* **Keep the `SPDX-License-Identifier: MIT` headers** on every kit file you copy.
  They mark these files as the kit's MIT boilerplate; do not replace them with a
  personal copyright line. (The infra copyright gate exempts SPDX-tagged files.)
* If DEST is not empty, list what you'd overwrite and ask before clobbering.

## Universal steps (every stack)

1. **Copyright holder — ask before writing any owned file.** Do this *first*,
   as an interview question, before generating any source. This repo's own
   files must carry a `Copyright (c) <year> <holder>` header, and the header gate
   (`copyright_test.go` / `test_copyright.py` / `copyright.test.ts` /
   `copyright_checks.test.ts`) **fails hard the moment a non-SPDX source file
   exists while the holder is unset**. An agent normally generates real code
   during scaffolding — not just the SPDX-tagged kit files — so the holder is
   needed up front, long before `setup_dev.sh` runs.
   * Ask the user: *"Who is the copyright holder for this project's source?
     (e.g. `Jane Doe` or `Acme Inc`)"* You may *propose* a default inferred from
     the environment (git `user.name`/`user.email`, an existing `LICENSE`), but
     **confirm it — never assume it silently.** A guessed name is a real
     ownership decision made on the user's behalf.
   * Write the confirmed value to `DEST/.copyright-holder` (the user commits it).
   * Every non-SPDX file you author from here on (including any code/schema you
     generate) must carry `Copyright (c) <current-year> <holder>`. Kit files you
     copy keep their `SPDX-License-Identifier: MIT` headers untouched.
   This is belt-and-suspenders: `setup_dev.sh` re-prompts only if
   `.copyright-holder` is still missing, so setting it now makes setup a no-op
   for it later. If the user declines to give a holder, do **not** invent one —
   either author only SPDX-tagged files, or leave code generation to the user and
   let `setup_dev.sh` collect the holder.
2. **CLAUDE.md** — copy `KIT/<stack>/CLAUDE_<stack>.md` → `DEST/CLAUDE.md`. Then
   inline the shared discipline: read `KIT/CONVENTIONS.md` and prepend its body
   under a `## Engineering Conventions` heading (or replace the "see
   CONVENTIONS.md" pointer at the top with the actual content), so the new repo
   carries the conventions without depending on KIT.
3. **Stack files** — copy everything under `KIT/<stack>/` (except
   `CLAUDE_<stack>.md`, already handled) into `DEST/`, preserving structure:
   `tests/`, config files, `setup_dev.sh`, `.env_sample`, and any stack subdirs.
4. **.claude/** — create `DEST/.claude/` and copy:
   * `KIT/shared/claude/settings.json` → `DEST/.claude/settings.json`
   * `KIT/shared/claude/skills/review-code/` → `DEST/.claude/skills/review-code/`
5. **Git hooks** — copy the stack's pre-commit:
   * react: `KIT/react/githooks/pre-commit` → `DEST/.githooks/pre-commit`
   * go / python / infra: `KIT/shared/githooks/pre-commit` →
     `DEST/.githooks/pre-commit`, and replace the `TEST_CMD="..."` line with the
     stack's test command (see per-stack table below).
   Make it executable. (`setup_dev.sh` runs `git config core.hooksPath .githooks`.)
6. **.gitignore** — copy `KIT/shared/gitignore.base` → `DEST/.gitignore`.
7. **Make scripts executable** — `chmod +x DEST/setup_dev.sh` and the pre-commit.
8. **Report next steps** (see bottom).

## Per-stack details

Every stack ships a license/copyright gate (`copyright_test.go`,
`test_copyright.py`, `copyright.test.ts`, infra's `copyright_checks.test.ts`):
it exempts SPDX-tagged files (the kit's own) and requires the user's own files to
carry a `Copyright (c) <year> <holder>` header, where the holder comes from the
committed `.copyright-holder` file — written during scaffolding (universal step 1)
and re-confirmed by `setup_dev.sh` if still missing. A scaffold that ships only
SPDX-tagged kit files is green even before the holder is set; the gate bites as
soon as you (or the scaffolding agent) add a non-SPDX source file, which is
exactly why step 1 collects the holder up front.

### go
* Copy: `CLAUDE_go.md`, `go.mod`, `Makefile`, `setup_dev.sh`, `.env_sample`,
  `staticcheck.conf`, `.sqlfluff`,
  `tests/` (`quality_test.go`, `copyright_test.go`, plus `setup_test.go` +
  `schema_test.go`, which are `//go:build itest` tagged). Also copy
  `KIT/shared/scripts/pg_setup.sh` → `DEST/scripts/pg_setup.sh` (sourced by
  `setup_dev.sh`; keep its `SPDX` header, no `chmod` needed) and
  `KIT/go/scripts/rename_module.sh` → `DEST/scripts/rename_module.sh`
  (`chmod +x`; keep its `SPDX` header — it sets/renames the module path).
* **Module path — interview the user.** The module path is baked into every
  `internal/...` import the moment you generate layered code, so decide it up
  front (like the copyright holder and DB name). The shipped `go.mod` carries the
  sentinel `example.com/app`. Ask which form the user wants:
    1. `github.com/<org>/<DEST-dir>` — ask for `<org>`; host defaults to
       `github.com` (let them override). **Recommended.**
    2. `<DEST-dir>` — a bare module path equal to the directory name (fine for a
       local/private module until a remote exists).
    3. A custom module path they supply verbatim.
    4. Skip — leave the `example.com/app` sentinel and name it later.
  For 1–3, set the chosen value (run `DEST/scripts/rename_module.sh <path>`, or —
  before any code exists — just edit the `module` line in `go.mod`) and use it as
  the import prefix for **all** Go code you generate. For 4, leave the sentinel,
  generate imports against `example.com/app`, and add the rename line to the
  final report (below). The choice is reversible anytime:
  `scripts/rename_module.sh <new/path>` rewrites `go.mod` + every import and
  re-runs `go mod tidy`, and `setup_dev.sh` re-prompts whenever the sentinel is
  still in place.
* **Database name — interview the user.** Like the copyright holder, ask up front:
  *"What should the local Postgres database be named?"* Propose the `DEST`
  directory name as the default (the same value `setup_dev.sh` would fall back to),
  but confirm it. Substitute the chosen name for `app` in the `PG_URL`
  path of the copied `.env_sample` (leave the `user:password` placeholder — that's
  what tells `setup_dev.sh` to provision the role). This is belt-and-suspenders:
  if you skip it, `setup_dev.sh` derives the default from the directory and prompts.
* pre-commit TEST_CMD: `go test ./tests/ -timeout 120s`
* Notes: file-based gates run with plain `go test ./tests/`. DB-backed tests need
  `PG_URL` and run with `go test -tags itest ./tests/`. The user adds
  `create_tables.sql` and code under `internal/`. On first run `setup_dev.sh`
  provisions a Postgres role + database for the current user from `PG_URL` and
  rewrites the `PG_URL` line in `.env`; for a role that already exists it verifies
  or prompts for the existing password and, only if none works (e.g. a peer-auth
  role with no TCP password), offers to generate and set one with explicit consent.

### python
* Copy: `CLAUDE_python.md`, `setup_dev.sh`, `requirements.txt`,
  `pyrightconfig.json`, `pytest.ini`, `vulture_whitelist.py`, `.env_sample`,
  `tests/` (`test_no_dead_code.py`, `test_copyright.py`).
* pre-commit TEST_CMD: `.venv/bin/pytest -q`
* Notes: edit `SCAN_PATHS` in `test_no_dead_code.py` to your package layout.

### react
* Copy: `CLAUDE_react.md`, `setup_dev.sh`, `package.json`, `eslint.config.js`,
  `jest.config.js`, `playwright.config.ts`, `vite.config.ts`, `knip.json`,
  `tsconfig*.json`, `.env_sample`, `tests/` (`npmAudit.test.ts`,
  `licenses.test.ts`, `copyright.test.ts`), and the minimal `src/` stubs
  (`setupGlobals.ts`, `setupTests.ts`, `__mocks__/fileMock.cjs`).
* pre-commit: use `KIT/react/githooks/pre-commit` verbatim (stamp-file variant).
* Notes: `npm test` runs the license, npm-audit, and copyright gates after
  `npm install`.

### infra
* Copy: `CLAUDE_infra.md`, `setup_dev.sh`, `package.json`, `jest.config.js`,
  `tsconfig.json`, `cdk.json`, `.env_sample`, `scripts/lib.sh`, `tests/`
  (`lint_checks.test.ts`, `copyright_checks.test.ts`, `security_checks.test.ts`,
  and `cdk_nag_suppressions.test.ts.template` — leave the `.template` extension
  so it stays inactive until the user has a stack).
* pre-commit TEST_CMD: `npm test`
* Notes: the user adds `bin/app.ts` and `lib/`.

## Final report to the user

End by printing this next-steps block **verbatim** — substitute the real `DEST`
path and the stack's run command from the table below, and change nothing else:

```
cd <DEST>
git init            # if not already a repo
./setup_dev.sh      # installs deps & tooling, wires hooks, sets up the local dev env (.env, DB, etc.)
<run-command>       # start the app
```

Stack run command:

| Stack  | `<run-command>` |
|---|---|
| go     | `go run .` |
| react  | `npm run dev` |
| infra  | `npm run cdk -- synth`  (after you add `bin/app.ts`) |
| python | `python -m <your_package>`  (your service's entry point) |

**Keep the list to exactly these steps** — with one exception: if the user chose
to skip the go module path (option 4 above), insert this line immediately after
`git init`, so imports resolve before `setup_dev.sh` builds:

```
./scripts/rename_module.sh github.com/<org>/<DEST>   # set your real module path
```

(`setup_dev.sh` also re-prompts for it when the sentinel is still in place, so
this is belt-and-suspenders.) Otherwise: Do NOT add manual setup such as
`createdb`/`psql`, `create_tables.sql`, editing `.env`, or exporting `PG_URL` —
provisioning the database/user, applying the schema, and writing a working `.env`
are `setup_dev.sh`'s job. If a first run needs something that isn't covered, the
fix is to add it to `setup_dev.sh`, **not** to append a manual step here. (A
prior run leaked a manual `psql … create_tables.sql` line into the next-steps
because `setup_dev.sh` wasn't provisioning the DB — that's a setup bug, not a
next-step.)

For infra, note the copyright gate exempts the kit's SPDX-tagged files and only
requires a `Copyright (c) <year> <holder>` header on the user's *own* source. The
holder is read from `.copyright-holder` (collected in universal step 1, with
`setup_dev.sh` as a fallback); a scaffold of SPDX-only kit files is green until
the user adds their own un-headered files.
