#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Rewrite the Go module path across go.mod and every import, then re-tidy.
#
# Use this when the module path was left as the scaffold sentinel
# (example.com/app) or needs to change later — e.g. the GitHub org/repo name was
# decided after scaffolding. Solves the chicken-and-egg of having to choose an
# import path before the remote repo exists: scaffold against the sentinel now,
# rename in one shot whenever the real path is known.
#
# Idempotent: re-running with the current path is a no-op.
#
# Usage: scripts/rename_module.sh <new/module/path>
#   e.g. scripts/rename_module.sh github.com/acme/raceday-backend
set -euo pipefail

cd "$(dirname "$0")/.."

NEW_PATH="${1:-}"
if [ -z "$NEW_PATH" ]; then
  echo "usage: scripts/rename_module.sh <new/module/path>" >&2
  echo "  e.g. scripts/rename_module.sh github.com/acme/$(basename "$(pwd)")" >&2
  exit 2
fi

if [ ! -f go.mod ]; then
  echo "error: go.mod not found in $(pwd)" >&2
  exit 1
fi

OLD_PATH="$(awk '/^module /{print $2; exit}' go.mod)"
if [ -z "$OLD_PATH" ]; then
  echo "error: could not read the current module path from go.mod" >&2
  exit 1
fi

if [ "$OLD_PATH" = "$NEW_PATH" ]; then
  echo "module path is already '$NEW_PATH'; nothing to do."
  exit 0
fi

echo "Renaming module: $OLD_PATH -> $NEW_PATH"

# 1) The module declaration in go.mod. '|' is a safe delimiter: it cannot appear
#    in a module path.
sed -i "s|^module .*|module $NEW_PATH|" go.mod

# 2) Import prefixes across all Go sources.
files="$(grep -rl --include='*.go' "$OLD_PATH" . || true)"
if [ -n "$files" ]; then
  # shellcheck disable=SC2086
  sed -i "s|$OLD_PATH|$NEW_PATH|g" $files
  echo "  rewrote imports in:"
  # shellcheck disable=SC2086
  printf '    %s\n' $files
else
  echo "  no Go files reference the old path yet"
fi

# 3) Re-resolve modules so go.sum and the build cache match the new path.
if command -v go >/dev/null 2>&1; then
  echo "Running go mod tidy..."
  go mod tidy
else
  echo "warn: go not on PATH; run 'go mod tidy' yourself" >&2
fi

echo "Done. Module path is now '$NEW_PATH'."
