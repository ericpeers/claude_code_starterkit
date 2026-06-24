#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# install.sh — copy the auth module into a project scaffolded from this kit.
#
# Usage:  go/optional/auth/install.sh [DEST]
#   DEST   project root (a Go scaffold with go.mod). Defaults to $PWD.
#
# What it does (idempotent):
#   1. Copies internal/{middleware,models,services,handlers,repository}/*.go into
#      DEST/internal/ and rewrites the import prefix to DEST's module path.
#   2. Copies bin/* and scripts/setup_admin.sh into DEST, appends env.snippet to
#      DEST/.env_sample.
#   3. Installs the schema as DEST/create_tables.sql if absent (else prints the
#      path to merge).
#   4. Runs `go mod tidy` in DEST.
# After it runs, do the two wiring edits described in README.md (main.go + setup_dev.sh).

set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
DEST="${1:-$PWD}"
DEST="$(cd "$DEST" && pwd)"

[ -f "$DEST/go.mod" ] || { echo "ERROR: $DEST has no go.mod — point install.sh at a Go scaffold" >&2; exit 1; }

MODULE="$(awk '/^module /{print $2; exit}' "$DEST/go.mod")"
[ -n "$MODULE" ] || { echo "ERROR: could not read module path from $DEST/go.mod" >&2; exit 1; }
OLD_PREFIX="example.com/app/optional/auth/internal"
NEW_PREFIX="${MODULE}/internal"

echo "==> Installing auth module into $DEST (module: $MODULE)"

# 1. Go sources -> DEST/internal/..., rewriting the import prefix in one pass
#    (collapses optional/auth/internal -> internal AND applies the real module path).
for sub in middleware models services handlers repository; do
  mkdir -p "$DEST/internal/$sub"
  for f in "$SRC/internal/$sub"/*.go; do
    [ -e "$f" ] || continue
    out="$DEST/internal/$sub/$(basename "$f")"
    sed "s#${OLD_PREFIX}#${NEW_PREFIX}#g" "$f" > "$out"
    echo "  internal/$sub/$(basename "$f")"
  done
done

# 1b. Test (ships as a .tmpl so the kit never compiles it; rendered into tests/).
if [ -f "$SRC/tests/auth_test.go.tmpl" ]; then
  mkdir -p "$DEST/tests"
  sed "s#${OLD_PREFIX}#${NEW_PREFIX}#g" "$SRC/tests/auth_test.go.tmpl" > "$DEST/tests/auth_test.go"
  echo "  tests/auth_test.go"
fi

# 2. Scripts + env.
mkdir -p "$DEST/bin" "$DEST/scripts"
cp "$SRC/bin/login" "$DEST/bin/login"; chmod +x "$DEST/bin/login"
cp "$SRC/bin/set_admin_password.py" "$DEST/bin/"; chmod +x "$DEST/bin/set_admin_password.py"
cp "$SRC/bin/encrypt_admin_passwd.py" "$DEST/bin/"; chmod +x "$DEST/bin/encrypt_admin_passwd.py"
cp "$SRC/scripts/setup_admin.sh" "$DEST/scripts/setup_admin.sh"
echo "  bin/login, bin/*.py, scripts/setup_admin.sh"

if [ -f "$DEST/.env_sample" ] && ! grep -q '^ADMIN_EMAIL=' "$DEST/.env_sample"; then
  cat "$SRC/env.snippet" >> "$DEST/.env_sample"
  echo "  appended auth vars to .env_sample"
fi

# 3. Schema.
if [ ! -f "$DEST/create_tables.sql" ]; then
  cp "$SRC/schema/auth_tables.sql" "$DEST/create_tables.sql"
  echo "  installed create_tables.sql (users table)"
else
  echo "  NOTE: create_tables.sql already exists — merge the auth schema into it:"
  echo "        $SRC/schema/auth_tables.sql"
fi

# 4. Resolve modules (pulls golang-jwt/jwt/v5 and golang.org/x/crypto).
( cd "$DEST" && go mod tidy ) && echo "  go mod tidy"

cat <<EOF

Auth module installed. Two manual wiring edits remain (see README.md):
  1. main.go      — wire middleware.ValidateUser + the /auth routes, and add the
                    BearerAuth swagger security definition.
  2. setup_dev.sh — after the schema-apply phase, add:
        [ -f scripts/setup_admin.sh ] && source scripts/setup_admin.sh
Then run ./setup_dev.sh and bin/login.
EOF
