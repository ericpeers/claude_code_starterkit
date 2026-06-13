#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Idempotent dev-environment bootstrap for the React stack.
# Safe to re-run: each phase checks before doing work.
set -euo pipefail

cd "$(dirname "$0")"

log()  { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- Phase 1: Node ---------------------------------------------------------
log "Checking Node.js"
if ! have node; then
  warn "Node.js not found. Install Node 20 LTS+ (https://nodejs.org) and re-run."
  exit 1
fi
node --version

# --- Phase 2: dependencies -------------------------------------------------
log "Installing npm dependencies"
npm install

# --- Phase 3: Playwright browser ------------------------------------------
log "Installing Playwright Chromium"
npx playwright install chromium || warn "playwright install failed; e2e tests will be skipped"

# --- Phase 4: .env from template ------------------------------------------
log "Setting up .env"
if [ ! -f .env ]; then
  cp .env_sample .env
  echo "  created .env from .env_sample — fill in real values"
else
  echo "  .env already exists; leaving it untouched"
fi

# --- Phase 5: copyright holder ---------------------------------------------
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

# --- Phase 6: git hooks ----------------------------------------------------
log "Wiring git hooks"
if [ -d .git ]; then
  git config core.hooksPath .githooks
  [ -f .githooks/pre-commit ] && chmod +x .githooks/pre-commit
  echo "  core.hooksPath -> .githooks"
else
  warn "not a git repository; run 'git init' then re-run to wire hooks"
fi

# --- Phase 7: tests --------------------------------------------------------
log "Running unit tests"
npm test

cat <<'EOF'

Setup complete.
  - Start the dev server with:  npm run dev
  - Run e2e tests with:         npm run test:e2e
EOF
