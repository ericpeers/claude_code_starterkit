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

## Logging

One logger for the whole app: `logrus`, configured once at startup, used
everywhere via the package logger (`log "github.com/sirupsen/logrus"`).

* **Set the formatter and level once in `main`** before doing any work, so the
  startup banner and every later line share one format. Map `LOGLEVEL`
  (debug|info|warn|error from config) onto `log.SetLevel`; treat an unrecognized
  value as a fatal config error rather than silently defaulting.
* **Route Gin's request logs through logrus** with `middleware.GinLogger()` (it
  picks the level from the response status: 5xx→Error, 4xx→Warn, else Info), so
  request lines obey the same `LOGLEVEL` filter. Build the router with
  `gin.New()` + `gin.Recovery()`, never `gin.Default()` (which installs Gin's
  own stdout logger and produces a second, differently-formatted stream).
* Log through the `log` package functions (`log.Infof`, `log.Errorf`), not
  `fmt.Print*`; never the standard library `log` package.

## Swagger / API docs

The OpenAPI surface is **generated from handler annotations** — one source of
truth, never a hand-maintained spec.

* Annotate `main()` (title/version/basepath/security) and each handler with
  `swag` comments. Generate the spec with `make docs` (`swag init` → `./docs`).
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

* **One suite, run every time.** There are no build tags or a separate DB-only
  test class: `go test ./tests/` (or `make test`) runs the file-based gates and
  the DB-backed tests together, always. Don't reintroduce a split (`-tags itest`,
  a `test_itest` target) — if a test needs the database, it skips itself when no
  DB is configured rather than living behind a tag.
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

### License-header gate

`tests/copyright_test.go` exempts files carrying `SPDX-License-Identifier:` (the
kit's own boilerplate) and requires your `.go` files to have a
`Copyright (c) <year> <holder>` header, where the holder is read from the
committed `.copyright-holder` file (`setup_dev.sh` writes it). A fresh scaffold is
green; the gate bites once you add your own un-headered code.

## Environment

Load all configuration through `config/`. Document every variable in
`.env_sample` (names only, never real values). `.env` is gitignored.
