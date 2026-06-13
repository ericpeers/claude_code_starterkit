#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Idempotent dev-environment bootstrap for the Python stack.
# Safe to re-run: each phase checks before doing work.
set -euo pipefail

cd "$(dirname "$0")"

log()  { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

PY=python3
MIN_MINOR=12  # require Python 3.12+

# --- Phase 1: Python version ---------------------------------------------
log "Checking Python"
if ! have "$PY"; then
  warn "python3 not found. Install Python 3.${MIN_MINOR}+ and re-run."
  exit 1
fi
"$PY" - <<EOF
import sys
maj, minr = sys.version_info[:2]
if (maj, minr) < (3, ${MIN_MINOR}):
    sys.exit(f"Python 3.${MIN_MINOR}+ required, found {maj}.{minr}")
print(f"Python {maj}.{minr} OK")
EOF

# --- Phase 2: virtualenv --------------------------------------------------
log "Creating virtualenv (.venv)"
if [ ! -d .venv ]; then
  "$PY" -m venv .venv
  echo "  created .venv"
else
  echo "  .venv already exists"
fi
# shellcheck disable=SC1091
source .venv/bin/activate

# --- Phase 3: dependencies ------------------------------------------------
log "Installing dependencies"
pip install --upgrade pip >/dev/null
pip install -r requirements.txt

# Optional: browser automation. Uncomment if your project uses Playwright.
# pip install playwright && playwright install chromium

# --- Phase 4: .env from template -----------------------------------------
log "Setting up .env"
if [ ! -f .env ]; then
  cp .env_sample .env
  echo "  created .env from .env_sample — fill in real values"
else
  echo "  .env already exists; leaving it untouched"
fi

# --- Phase 5: copyright holder --------------------------------------------
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

# --- Phase 6: git hooks ---------------------------------------------------
log "Wiring git hooks"
if [ -d .git ]; then
  git config core.hooksPath .githooks
  [ -f .githooks/pre-commit ] && chmod +x .githooks/pre-commit
  echo "  core.hooksPath -> .githooks"
else
  warn "not a git repository; run 'git init' then re-run to wire hooks"
fi

# --- Phase 7: tests -------------------------------------------------------
log "Running tests"
pytest -q

cat <<'EOF'

Setup complete. Activate the venv in new shells with:  source .venv/bin/activate
EOF
