#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# setup_admin.sh — auth-module setup phase, sourced by setup_dev.sh AFTER the
# schema is applied. Generates ADMIN_PASS on first run and sets it on the seeded
# admin row. Idempotent: once ADMIN_PASS is a real value it is reused, and a
# re-run just re-hashes and re-sets the same password.
#
# Wire it into setup_dev.sh by adding, after the schema-apply phase:
#   [ -f scripts/setup_admin.sh ] && source scripts/setup_admin.sh

printf '\n\033[1;34m==> %s\033[0m\n' "Setting up admin account (auth module)"

_AUTH_WARN() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
_AUTH_DONE() { return 0 2>/dev/null || exit 0; }

_DOTENV=".env"
if [ ! -f "$_DOTENV" ]; then
  _AUTH_WARN "no .env found — skipping admin setup"
  _AUTH_DONE
fi

# Read a value from .env by key. A missing key is an expected case (the caller
# checks for an empty result below), so grep's stderr is suppressed deliberately.
_auth_get() {
  grep -E "^(export )?$1=" "$_DOTENV" 2>/dev/null | head -1 | sed -E "s/^(export )?$1=//" | tr -d '"'
}

# Generate ADMIN_PASS if it is still the placeholder, mirroring setup_dev.sh's
# JWT_SECRET handling. Alphanumeric only so it needs no URL-encoding anywhere.
if grep -qE '^(export )?ADMIN_PASS=change-me$' "$_DOTENV" 2>/dev/null; then
  _PASS="$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom 2>/dev/null | head -c 24)"
  if [ -n "$_PASS" ]; then
    sed -i "s|^\(export \)\{0,1\}ADMIN_PASS=.*|ADMIN_PASS=${_PASS}|" "$_DOTENV"
    chmod 600 "$_DOTENV" 2>/dev/null || true
    echo "  generated ADMIN_PASS"
  else
    _AUTH_WARN "could not generate ADMIN_PASS — set it in .env manually"
  fi
fi

_EMAIL="$(_auth_get ADMIN_EMAIL)"
_PASS="$(_auth_get ADMIN_PASS)"
_PG="$(_auth_get PG_URL)"

if [ -z "$_EMAIL" ] || [ -z "$_PASS" ] || [ -z "$_PG" ]; then
  _AUTH_WARN "ADMIN_EMAIL / ADMIN_PASS / PG_URL not all set in .env — skipping admin password"
  _AUTH_DONE
fi

if ! command -v python3 >/dev/null 2>&1; then
  _AUTH_WARN "python3 not found — run bin/set_admin_password.py manually"
  _AUTH_DONE
fi

python3 bin/set_admin_password.py "$_EMAIL" "$_PASS" "$_PG" \
  || _AUTH_WARN "admin password not set — check that ADMIN_EMAIL exists in the users table"
