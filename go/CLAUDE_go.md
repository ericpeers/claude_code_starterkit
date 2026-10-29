# CLAUDE.md — Go stack

> Working discipline that applies to every stack lives in `CONVENTIONS.md`
> (failing-test-first, plan execution, verify-before-claiming, DDL confirmation,
> named functions over closures, etc.). The scaffold inlines the relevant parts
> into this file. This document adds Go-specific guidance.

## Architecture

A typical layout for an HTTP API backed by Postgres:

* `main.go` — entry point, dependency wiring, router setup
* `config/` — config struct; loads connection strings / secrets from env
* `internal/middleware/` — request middleware (auth, logging)
* `internal/models/` — data models and request/response DTOs
* `internal/handlers/` — HTTP handlers (thin; translate HTTP ↔ services)
* `internal/services/` — business logic
* `internal/repository/` — database operations (one file per aggregate)
* `tests/` — integration tests with an isolated ephemeral DB

### Dependency flow

```
handlers → services → repositories → db pool
                   → external API clients
```

Keep the arrows one-directional. Handlers never touch the pool directly;
repositories never contain business logic.

### Default HTTP stack

The kit standardizes on **Gin** for routing, **logrus** for logging, and
**swaggo** for API docs, and ships the wiring as building blocks:
`internal/middleware/gin_logger.go` and `internal/swagger/swagger.go`. Use them;
don't hand-roll a second logging or docs path. A `main.go` follows this shape:

```go
log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: "2006-01-02 15:04:05"})
setLogLevel(cfg.LogLevel)              // see Logging below

gin.SetMode(gin.ReleaseMode)
router := gin.New()                    // NOT gin.Default() — it adds Gin's own logger
router.Use(gin.Recovery(), middleware.GinLogger())
if cfg.EnableSwagger {                 // ENABLE_SWAGGER
    swagger.Register(router)
    log.Info("Swagger UI enabled at /swagger/index.html")
}
```

**Optional auth.** Need login + protected endpoints? The kit ships an opt-in
JWT + bcrypt-password module under `optional/auth/` (in the starter kit, not
copied into a scaffold by default). Run its `install.sh` to drop the middleware,
service, repository, `users` schema, and `bin/login` into place. See
`optional/auth/README.md` and the "Optional: auth module" section of `SCAFFOLD.md`.

## Logging

One logger for the whole app: `logrus`, configured once at startup, used
everywhere via the package logger (`log "github.com/sirupsen/logrus"`).

* **Set the formatter and level once in `main`** before doing any work, so the
  startup banner and every later line share one format. Map `LOGLEVEL`
  (debug|info|warn|error from config) onto `log.SetLevel`; treat an unrecognized
  value as a fatal config error rather than silently defaulting.
* **Route Gin's request logs through logrus** with `middleware.GinLogger()`, so
  request lines obey the same `LOGLEVEL` filter. It picks the level from the
  response status: 5xx→Error, 4xx→Warn, else Info. Build the router with
  `gin.New()` + `gin.Recovery()`, never `gin.Default()` (which installs Gin's
  own stdout logger and produces a second, differently-formatted stream).
* Log through the `log` package functions (`log.Infof`, `log.Errorf`), not
  `fmt.Print*`; never the standard library `log` package.

## Swagger / API docs

The OpenAPI surface is **generated from handler annotations** — one source of
truth, never a hand-maintained spec.

* Annotate `main()` (title/version/basepath/security) and each handler with
  `swag` comments. Generate the spec with `make docs` (`go tool swag init` →
  `./docs`).
* Blank-import the generated package in `main.go` so its `init()` registers the
  spec: `import _ "<your-module>/docs"`. Mount the UI with
  `swagger.Register(router)`, gated on `cfg.EnableSwagger` (`ENABLE_SWAGGER`) so
  it is off in production.
* `tests/swagger_naming_test.go` lints the generated `docs/swagger.json`: URL
  path segments must be **kebab-case**, JSON fields and query/body/form/path
  params **snake_case**. The gate skips until you generate docs, then enforces.
  Regenerate docs (`make docs`) after changing annotations so the gate sees the
  current surface.

## Function design

When a function computes intermediate data that could be useful elsewhere:

* **Extract it** into a separate public function rather than returning it as a
  byproduct.
* **Pass it in** to dependent functions rather than having them recompute it.

For example, a `ComputeLineItemTotals()` that is separate from `ComputeOrderTotal()`
lets the line-item totals be reused (receipt display, other rollups) without
recomputation.

## Repository method naming

Follow a consistent pattern based on cardinality and return shape:

| Pattern | Cardinality | Returns | Example |
|---|---|---|---|
| `GetFoo` | Single entity (by ID) | `[]T` or `*T` | `GetOrderItems` |
| `GetFooMulti` | Many entities, full records | `map[id][]T` | `GetOrderItemsMulti` |

`Multi` methods return the complete per-key record set for every id in one
`ANY($1)` query. Use them when downstream code iterates over individual records.

## Repository table ownership

Each file in `internal/repository/` owns specific tables and is the only file
allowed to **mutate** them. If you need data from a table owned by another file,
call that file's methods instead. **Exception:** read-only JOINs for lookups are
allowed (whitelist them explicitly).

`TestRepositoryTableOwnership` in `tests/quality_test.go` enforces this
mechanically — it parses backtick SQL strings and checks every referenced table
against an ownership map. Keep the map current as you add tables.

## Large result sets

When a query could return millions of rows (bulk export):

* **Stream via a callback, never accumulate.** A repository method serving a
  large export should accept a `func(row) error` and call it per row rather than
  building a `[]T`. Accumulation causes O(n) memory and GC pressure.
* **Pre-fetch sparse side-tables into a map** and merge in Go rather than a
  per-row LEFT JOIN across the large table.
* **Wrap the HTTP writer** in `bufio.NewWriterSize` (e.g. 256KB) for large
  streaming responses.

## Testing

* **One suite, run every time.** There are no build tags and no separate DB-only
  test class. `go test ./tests/` (or `make test`) always runs the file-based
  gates and the DB-backed tests together. Don't reintroduce a split (`-tags itest`,
  a `test_itest` target). A test that needs the database skips itself when no DB
  is configured, rather than living behind a tag.
* Every feature gets a test in `tests/`, covering both error conditions and
  correctness.
* DB-backed tests run against an **isolated ephemeral database** created and
  dropped by `TestMain` (see `tests/setup_test.go`). A safety guard aborts if the
  connected DB looks like production. The production database is never touched.
* **The database is required once you have a schema** — it is not optional that
  silently skips. The rule (in `tests/setup_test.go`):
  * no `create_tables.sql` yet (fresh scaffold) → no DB-backed tests exist, so the
    run skips DB setup and stays green;
  * `create_tables.sql` present but `PG_URL` unset → the run **fails** (a dropped
    or forgotten `PG_URL` must not turn the DB suite into a green no-op);
  * `SKIP_DB=1` → explicit opt-out for a lint-only run or a host without Postgres.
  Guard your own DB tests with `if testPool == nil { t.Skip(...) }` so they skip
  only on the legitimate opt-out paths.
* Do not rely on production data existing; create what your test needs.

### Running tests

```bash
go test ./tests/ -timeout 120s              # everything — file-based gates + DB-backed tests
SKIP_DB=1 go test ./tests/ -timeout 120s    # file-based gates only (no Postgres needed)
```

`PG_URL` comes from `.env`, which is loaded automatically — no need to source
anything. Once `create_tables.sql` exists, a reachable `PG_URL` is mandatory
unless you pass `SKIP_DB=1`.

### Comment-style gates

`tests/comment_style_test.go` holds three gates over comments and Markdown prose:
`TestNoBannedCommentWords`, `TestNoAbsoluteClaims`, and `TestCommentSentenceLength`.
They enforce the mechanical parts of *Comment style* in `CONVENTIONS.md`.
`tests/comment_length_test.go` adds four over Go source:

* `TestCommentBlockLength`: a comment block runs at most 6 lines.
* `TestFuncDocLength`: a function doc runs at most 4 prose lines, or 2 for a
  one-line body. Indented list items do not count.
* `TestSwaggerDescriptionLength`: a route's `@Description` is 1-3 sentences.
* `TestFieldCommentPlacement`: a struct field's comment goes at the end of its
  line when it fits in 120 columns. It also goes there when the other fields in
  the struct use end-of-line comments.

`tests/comment_scan_helpers_test.go` extracts the prose from Go, SQL, Python, and
Markdown. It leaves out code, string literals, fenced blocks, license headers,
and the swag annotations that build `swagger.json`.

The gates walk the tree rooted at the enclosing `go.mod`. In a scaffolded
project that is the project root, so they cover everything. In a repository
where the Go module sits in a subdirectory, they cover that subdirectory only,
and `TestProseScanCoversTrackedFiles` checks the same narrower set.

Each gate compares per-file hit counts against a baseline under
`tests/testdata/comment_style/`. A fresh scaffold ships those empty, so the gates
start enforcing on the first file you add. On an existing codebase, record what
is already there once and ratchet it down. A recorded count is a rule break the
gate lets pass, so record one only with a plan to remove it; `todo.md` tracks
that. The comparison is exact in both directions. A new hit fails the gate. A
cleaned-up hit fails it too, until the baseline drops to match. So rewrite the
prose first, then record the lower counts:

```bash
COMMENT_STYLE_BASELINE=write go test ./tests/ -run 'TestNoBannedCommentWords|TestNoAbsoluteClaims|TestCommentSentenceLength|TestCommentBlockLength|TestFuncDocLength|TestSwaggerDescriptionLength|TestFieldCommentPlacement'
```

`COMMENT_STYLE_REPORT=1` with `-v` lists every hit, baselined or not, and drops
the per-file cap on the failure message. Raising a baseline count to land new
prose defeats the gate; rewrite the prose instead.

Two lists in `comment_style_helpers_test.go` are yours to extend: `styleExemptPhrases`,
for domain vocabulary that carries a banned word innocently, and
`bannedCommentWords` itself. `TestProseScanCoversTrackedFiles` checks the walk
reaches every tracked file, so a pruned directory does not quietly shrink a
baseline. `comment_scan_cases_test.go` covers the scanners, the block splitter,
and the baseline comparison itself.

### Static-analysis gates

The linters are declared in the `tool` block of `go.mod` and run through
`go tool <name>`, at the version `go.mod` names. Nothing is installed globally.
`setup_dev.sh` builds them once with `go build tool`. Upgrade one with
`go get -tool <pkg>@latest`, then `go mod tidy`.

`tests/static_analysis_test.go` runs `staticcheck` (settings in
`staticcheck.conf`), `go vet`, `gofmt -l`, `go mod tidy -diff`, `ineffassign`,
`unparam -exported`, and `gosec`. Suppress a gosec finding with `#nosec` and a
reason at the call site. It also holds four deeper gates:

* `TestDeadcode`: no function is unreachable from `main`.
* `TestInternalPackagesReachable`: `main` imports every package under `internal/`.
* `TestNoUncalledExportedMethods`: every exported method has a use. deadcode
  counts any method of a type stored in an interface as reachable.
* `TestGovulncheck`: no reachable vulnerability has a fix waiting, and each one
  without a fix is listed in `tests/vuln_allowlist.json`. `make fix_vulns` runs
  `scripts/fix_vulns.sh`. It upgrades what it can and offers to allowlist the
  rest with a reason. It needs `jq`.

The first two skip until the module has a `main` package. `deadcodeAllowlist`
keeps a function on purpose, with a reason. Packages under `optional/` stay out
of the reachability gates until `install.sh` copies them into `internal/`.

`TestGoVersionCurrent` fails when `go.mod` trails the newest Go series.
`TestGoToolsCurrent` fails when a tool trails its latest release by two minors.
Both live in `tests/tool_versions_test.go`. These two and `TestGovulncheck` need the network, and
can fail on the day an upstream release ships.

### Hygiene and layering gates

* `TestNoHackFixmeMarkers`: a `HACK FIXME` comment marks debug code that must
  not be committed. A bare `FIXME` is fine.
* `TestMaxLineLength`: production Go lines stay within 250 columns, with a tab as 4. Put
  `//no line_len check` on the line above one that resists wrapping.
* `TestNoBlankVariableDiscard`: no `_ = x` on a plain variable in production
  code. It only hides a dead variable.
* `TestNoTestFilesOutsideTestsDir`: every `_test.go` file lives in `tests/`.
* `TestSharedHelpersLiveInHelperFiles`: a test-package name used by a second
  file is declared in `setup_test.go` or a `<topic>_helpers_test.go` file.
* `TestTestsRunOnlyReadOnlyGit`: tests run only `git ls-files`, `log`, and
  `check-ignore`. The pre-commit hook runs the suite, and a git write from
  there lands in the repository being committed.
* `TestGitignoreCoversDatabaseDumps` and `TestNoDatabaseDumpsTracked`: dump and
  archive files are gitignored, and none is tracked. The patterns include
  `*.dump`, `*.sql.gz`, `*.sqlite`, and `*.tar.gz`. Both skip outside a git work
  tree.
* `TestNegatedAssignmentGuardsSetPipefail`: a shell script that negates a
  pipeline assignment, as in `if ! x="$(a | b)"`, sets `pipefail`. Without it
  the guard sees only the status of `b`.
* `TestGuardedRegions`: code between `// guard:begin <name>` and
  `// guard:end <name>` matches its hash in `tests/testdata/guards.txt`.
  Comments and whitespace do not count. Only the human edits the manifest.
  `GUARD_HASH_REPORT=1` with `-v` prints the current hashes.
* `TestNoPgxImportsAboveRepository`: `internal/services`, `handlers`, and
  `models` do not import pgx. It skips until one of them exists.
* `TestNoRawErrorTextInResponses`: no error text reaches a Gin reply. The scan
  follows locals, parameters, and helper returns. Log the error and reply with
  text written for the caller.

Each detector has a fixture-based self-test beside it. A broken detector then
fails its self-test instead of passing the tree.

### License-header gate

`tests/copyright_test.go` exempts files carrying `SPDX-License-Identifier:`,
which is the kit's own boilerplate. It requires your `.go` files to have a
`Copyright (c) <year> <holder>` header. The holder is read from the
committed `.copyright-holder` file (`setup_dev.sh` writes it). A fresh scaffold is
green; the gate bites once you add your own un-headered code.

## Environment

Load all configuration through `config/`. Document every variable in
`.env_sample` — names only, no values. `.env` is gitignored.
