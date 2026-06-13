#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# lib.sh -- Shared helpers sourced by other scripts. Do not invoke directly.
#
# The sourcing script must define these logging helpers before calling the
# functions here:  ok, info, fail
set -euo pipefail

# read_local_env_key FILE KEY -- echo the value of KEY=... from a dotenv-style
# file, stripping one matching pair of surrounding quotes.
read_local_env_key() {
  local file="$1" key="$2"
  local val
  [[ -r "$file" ]] || return 0  # no readable file: key is simply unset
  # `|| true` keeps a no-match (grep exit 1) or SIGPIPE-from-head from tripping
  # `set -e` in the caller; a missing key must yield empty, not abort the script.
  val=$(grep -E "^(export )?${key}=" "$file" \
    | head -1 \
    | sed -E "s/^(export )?${key}=//" || true)
  if [[ "$val" =~ ^\"(.*)\"$ ]] || [[ "$val" =~ ^\'(.*)\'$ ]]; then
    val="${BASH_REMATCH[1]}"
  fi
  printf '%s' "$val"
}

# check_git_status DIR LABEL -- fail if DIR has uncommitted or untracked changes.
# Use as a pre-flight gate before deploying.
check_git_status() {
  local dir="$1"
  local label="$2"
  info "Checking $label files are committed to git ($dir)"
  if [[ ! -d "$dir" ]]; then
    fail "$label directory not found at $dir"
  fi
  if [[ -n $(cd "$dir" && git status --porcelain) ]]; then
    echo ""
    (cd "$dir" && git status)
    fail "Uncommitted changes or untracked files found in $label. Commit or stash them before deploying."
  fi
  ok "$label git status is clean"
}
