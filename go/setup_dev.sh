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
# gosec and unparam are optional; uncomment if you want them:
# install_tool gosec        github.com/securego/gosec/v2/cmd/gosec@latest
# install_tool unparam      mvdan.cc/unparam@latest

# --- Phase 3: .env from template -----------------------------------------
log "Setting up .env"
if [ ! -f .env ]; then
  cp .env_sample .env
  echo "  created .env from .env_sample — fill in real values"
else
  echo "  .env already exists; leaving it untouched"
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
log "Running file-based quality tests (no DB required)"
go test ./tests/ -timeout 120s

cat <<'EOF'

Setup complete.
  - Add your schema as create_tables.sql and code under internal/.
  - For DB-backed tests:  PG_URL=... go test -tags itest ./tests/
EOF
