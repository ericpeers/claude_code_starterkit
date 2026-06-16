#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Idempotent dev-environment bootstrap for the Go stack.
# Safe to re-run: each phase checks before doing work.
set -euo pipefail

cd "$(dirname "$0")"

log()  { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- Phase 1: Go toolchain ------------------------------------------------
log "Checking Go toolchain"
if ! have go; then
  warn "Go is not installed. Install it from https://go.dev/dl/ and re-run."
  exit 1
fi
go version

# --- Phase 2: Go developer tools -----------------------------------------
log "Installing Go developer tools"
install_tool() { # name install-path
  if [ ! -x "$(go env GOPATH)/bin/$1" ]; then
    echo "  installing $1"
    go install "$2"
  else
    echo "  $1 already present"
  fi
}
install_tool staticcheck   honnef.co/go/tools/cmd/staticcheck@latest
install_tool govulncheck   golang.org/x/vuln/cmd/govulncheck@latest
install_tool deadcode      golang.org/x/tools/cmd/deadcode@latest
# swag generates the Swagger/OpenAPI spec from handler annotations (make docs).
install_tool swag          github.com/swaggo/swag/cmd/swag@latest
# gosec and unparam are optional; uncomment if you want them:
# install_tool gosec        github.com/securego/gosec/v2/cmd/gosec@latest
# install_tool unparam      mvdan.cc/unparam@latest

# --- Phase 3: .env from template -----------------------------------------
log "Setting up .env"
if [ ! -f .env ]; then
  cp .env_sample .env
  echo "  created .env from .env_sample — fill in real values"
else
  echo "  .env already exists; leaving existing values untouched"
fi
# Replace the shipped JWT_SECRET placeholder with a generated secret so it never
# reaches a running server. Only the placeholder line is rewritten, so a real
# secret already in .env is preserved.
if grep -qE '^(export )?JWT_SECRET=change-me$' .env 2>/dev/null; then
  if have openssl; then
    sed -i "s|^\(export \)\{0,1\}JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" .env
    chmod 600 .env 2>/dev/null || true
    echo "  generated JWT_SECRET"
  else
    warn "openssl not found — JWT_SECRET left as placeholder; set it manually in .env"
  fi
fi

# --- Phase 3b: PostgreSQL role/database -----------------------------------
# Idempotently provision a Postgres role + db for the current developer and sync
# credentials into .env. Never resets an existing role's password (see helper).
log "Provisioning PostgreSQL"
PG_HELPER=""
for _c in "scripts/pg_setup.sh" "../shared/scripts/pg_setup.sh"; do
  [ -f "$_c" ] && { PG_HELPER="$_c"; break; }
done
if [ -n "$PG_HELPER" ]; then
  # shellcheck disable=SC1090
  source "$PG_HELPER"
  setup_postgres
else
  warn "scripts/pg_setup.sh not found — skipping Postgres provisioning"
fi

# --- Phase 3c: apply the schema -------------------------------------------
# Create the tables in the dev database. Idempotent and non-destructive: it only
# applies create_tables.sql when the public schema has no tables yet, so re-running
# never drops or rewrites existing tables. The clean DDL is kept free of
# IF NOT EXISTS (which CREATE TYPE can't express) by gating here.
# Skips cleanly on a fresh scaffold that has no create_tables.sql yet; rerun once
# you add one.
log "Applying database schema"
if ! have psql; then
  warn "psql not found — skipping schema apply; run 'psql \"\$PG_URL\" -f create_tables.sql' manually"
elif [ ! -f create_tables.sql ]; then
  echo "  create_tables.sql not present yet — skipping schema apply (add it and re-run)"
else
  SCHEMA_PG_URL="$(grep -E '^(export )?PG_URL=' .env 2>/dev/null | head -1 | sed -E 's/^(export )?PG_URL=//' | tr -d '"')"
  if [ -z "$SCHEMA_PG_URL" ]; then
    warn "PG_URL not found in .env — skipping schema apply"
  elif ! psql -w "$SCHEMA_PG_URL" -tAc 'SELECT 1' >/dev/null 2>&1; then
    warn "cannot connect to \$PG_URL — skipping schema apply (check .env credentials)"
  else
    EXISTING="$(psql -w "$SCHEMA_PG_URL" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'" 2>/dev/null || echo 0)"
    if [ "${EXISTING:-0}" != "0" ]; then
      echo "  schema already present (public schema has tables) — leaving it untouched"
    else
      if psql -w "$SCHEMA_PG_URL" -v ON_ERROR_STOP=1 -q -f create_tables.sql >/dev/null 2>&1; then
        echo "  applied create_tables.sql"
      else
        warn "failed to apply create_tables.sql — run 'psql \"\$PG_URL\" -f create_tables.sql' to see the error"
      fi
    fi
  fi
fi

# --- Phase 4: copyright holder --------------------------------------------
# Records the project's copyright holder so the header gate can enforce it on
# YOUR own source files. Kit-supplied files use SPDX headers and are exempt.
log "Configuring copyright holder"
if [ -f .copyright-holder ] && [ -s .copyright-holder ]; then
  echo "  copyright holder already set: $(cat .copyright-holder)"
else
  read -r -p "  Copyright holder for your files (e.g. 'Jane Doe' or 'Acme Inc'): " HOLDER
  if [ -n "${HOLDER}" ]; then
    printf '%s\n' "$HOLDER" > .copyright-holder
    echo "  wrote .copyright-holder (commit it)"
  else
    warn "no holder entered; set it later in .copyright-holder. The header gate needs it once you add non-SPDX files."
  fi
fi

# --- Phase 4b: Go module path ---------------------------------------------
# The module path is baked into every internal import. On a fresh scaffold it
# is still the 'example.com/app' sentinel if the module-path interview was
# skipped during scaffolding. Offer to set it now (idempotent: a no-op once it
# is no longer the sentinel). scripts/rename_module.sh rewrites go.mod + imports
# and re-tidies.
log "Checking Go module path"
MODULE_SENTINEL="example.com/app"
CURRENT_MODULE="$(awk '/^module /{print $2; exit}' go.mod 2>/dev/null || true)"
if [ "$CURRENT_MODULE" = "$MODULE_SENTINEL" ]; then
  read -r -p "  Module path is still the '$MODULE_SENTINEL' placeholder. Enter the real one (e.g. github.com/your-org/$(basename "$(pwd)")), blank to keep it: " MODULE_PATH
  if [ -n "${MODULE_PATH:-}" ]; then
    if [ -x scripts/rename_module.sh ]; then
      scripts/rename_module.sh "$MODULE_PATH"
    else
      warn "scripts/rename_module.sh not found; set 'module' in go.mod manually, then run 'go mod tidy'"
    fi
  else
    warn "module path left as '$MODULE_SENTINEL'; run scripts/rename_module.sh <path> when ready"
  fi
else
  echo "  module path: $CURRENT_MODULE"
fi

# --- Phase 5: git hooks ---------------------------------------------------
log "Wiring git hooks"
if [ -d .git ]; then
  git config core.hooksPath .githooks
  [ -f .githooks/pre-commit ] && chmod +x .githooks/pre-commit
  echo "  core.hooksPath -> .githooks"
else
  warn "not a git repository; run 'git init' then re-run to wire hooks"
fi

# --- Phase 6: dependencies + build ---------------------------------------
log "Resolving modules"
go mod tidy

log "Building"
go build ./... || warn "build failed — expected on an empty scaffold until you add main.go"

# --- Phase 7: tests -------------------------------------------------------
# One suite: file-based gates and DB-backed tests run together. A fresh scaffold
# (no create_tables.sql) is green with no DB; once a schema exists the DB is
# required (a missing PG_URL fails rather than skipping). By this point Phase 3b
# has written a working PG_URL to .env, so the DB tests run if a schema is present.
log "Running tests"
go test ./tests/ -timeout 120s

cat <<'EOF'

Setup complete.
  - If you added create_tables.sql, the schema has been applied to your dev database.
  - Add your code under internal/ (and create_tables.sql if you haven't yet, then re-run).
  - Build & run:  make build && ./app   (or: go run .)
  - Tests:        make test   (or: go test ./tests/ -timeout 120s) — runs everything every time
See README.md for the full workflow.
EOF
