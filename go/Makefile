.PHONY: all test build run docs setup_githooks fix_vulns

# `make test` runs the entire suite (file-based + DB-backed) in one pass — there
# is no separate DB-only test target by design (see the test target's comment).

all: docs test build

setup_githooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit

# One suite, everything every time: file-based gates and DB-backed tests run
# together. A fresh scaffold (no create_tables.sql) stays green with no DB, but
# once a schema exists a reachable PG_URL is required — a missing one fails rather
# than silently skipping (see tests/setup_test.go). SKIP_DB=1 forces a lint-only run.
test:
	go test ./tests/ -timeout 120s

# Regenerate the Swagger/OpenAPI spec from handler annotations into ./docs.
# swag runs through `go tool`, at the version declared in go.mod. main.go must
# blank-import the generated package (_ "<module>/docs") for the UI to serve it.
docs:
	go tool swag init --parseDependency --parseInternal -o docs

run:
	go run .

# Upgrade flagged dependencies, then triage the rest into
# tests/vuln_allowlist.json. Needs jq.
fix_vulns:
	bash scripts/fix_vulns.sh

# Version string: YYYY.MM.DD.<commits-today>-<short-hash>
DATE    := $(shell date +%Y.%m.%d)
HASH    := $(shell git rev-parse --short HEAD 2>/dev/null || echo nogit)
COUNT   := $(shell git rev-list --count --since="$$(date +%Y-%m-%d) 00:00:00" HEAD 2>/dev/null || echo 0)
VERSION := $(DATE).$(COUNT)-$(HASH)

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o app .
