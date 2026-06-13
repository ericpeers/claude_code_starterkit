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

* Every feature gets a test in `tests/`, covering both error conditions and
  correctness.
* Tests run against an **isolated ephemeral database** created and dropped by
  `TestMain` (see `tests/setup_test.go`). A safety guard aborts if the connected
  DB looks like production. The production database is never touched.
* Do not rely on production data existing; create what your test needs.

### Running tests

```bash
go test ./tests/ -timeout 120s              # file-based gates, no DB
PG_URL=... go test -tags itest ./tests/     # full suite incl. DB-backed tests
```

The `.env` file is loaded automatically — no need to source anything.

### License-header gate

`tests/copyright_test.go` exempts files carrying `SPDX-License-Identifier:` (the
kit's own boilerplate) and requires your `.go` files to have a
`Copyright (c) <year> <holder>` header, where the holder is read from the
committed `.copyright-holder` file (`setup_dev.sh` writes it). A fresh scaffold is
green; the gate bites once you add your own un-headered code.

## Environment

Load all configuration through `config/`. Document every variable in
`.env_sample` (names only, never real values). `.env` is gitignored.
