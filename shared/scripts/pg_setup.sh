#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# pg_setup.sh -- Shared helper sourced by a stack's setup_dev.sh. Do not invoke directly.
#
# Provides setup_postgres(): idempotently provision a PostgreSQL role + database
# for the current developer and reconcile credentials into .env.
#
# For an existing role this never SILENTLY resets the password: the role may have
# been created long ago and shared with other apps/clients, so an unprompted ALTER
# could break them. Instead it (1) accepts passwordless auth if the role already
# connects over TCP without one, (2) verifies any password already in .env or
# prompts for the one in use, and only as a last resort (3) offers -- with explicit
# consent -- to generate and set a password. That last step exists because a
# peer-auth role connects fine via the local socket (and from the psql cmdline)
# but has no password the app can use over TCP, where pg_hba usually demands one.
# A brand-new role always gets a freshly generated password.
#
# The sourcing script must define these helpers before calling, and must have
# cd'd into the directory that holds .env (the project root):
#   log()  -- section/info message
#   warn() -- warning to stderr
#   have() -- command-existence test (command -v "$1" >/dev/null)
set -euo pipefail

# _pg_parse_url URL -> fields joined by \x1f: user, pass, host, port, dbname.
# Inline Python (stdlib only): bash can't safely URL-decode a password. The
# unit-separator keeps an empty password (passwordless URL) from collapsing when
# the caller splits with `read` — read with IFS=$'\x1f' below.
_pg_parse_url() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import urlparse, unquote
p = urlparse(sys.argv[1])
print("\x1f".join([
    p.username or "",
    unquote(p.password) if p.password else "",
    p.hostname or "localhost",
    str(p.port or 5432),
    p.path.lstrip("/") or "app",
]), end="")
PY
}

# _pg_sanitize_dbname NAME -> a valid, lowercased PostgreSQL identifier
# (letters/digits/underscore, must not start with a digit, <= 63 chars).
_pg_sanitize_dbname() {
  local n
  n="$(printf '%s' "$1" | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C tr -c 'a-z0-9_' '_')"
  [[ "$n" =~ ^[a-z_] ]] || n="_${n}"
  printf '%s' "${n:0:63}"
}

# _pg_url USER PASS HOST PORT DB -> connection string (omits ":pass" when PASS empty).
_pg_url() {
  local u="$1" pw="$2" h="$3" port="$4" db="$5"
  if [ -n "$pw" ]; then
    printf 'postgres://%s:%s@%s:%s/%s' "$u" "$pw" "$h" "$port" "$db"
  else
    printf 'postgres://%s@%s:%s/%s' "$u" "$h" "$port" "$db"
  fi
}

# _pg_set_env KEY VALUE FILE -- replace an existing KEY=... line in FILE in place.
_pg_set_env() {
  local key="$1" val="$2" file="$3" esc
  # Escape sed replacement metacharacters (\ & and the | delimiter).
  esc=$(printf '%s' "$val" | sed -e 's/[|&\\]/\\&/g')
  sed -i "s|^\(export \)\{0,1\}${key}=.*|${key}=${esc}|" "$file"
}

# setup_postgres -- provision role + database and reconcile .env. Best-effort:
# warns and returns 0 (never aborts the caller) when Postgres/superuser access
# is unavailable, since file-based tests don't need a database.
setup_postgres() {
  local envfile=".env"
  [ -f "$envfile" ] || { warn "no .env present — skipping Postgres provisioning"; return 0; }
  have psql    || { warn "psql not found — skipping Postgres provisioning (install postgresql-client to enable)"; return 0; }
  have python3 || { warn "python3 not found — skipping Postgres provisioning"; return 0; }

  # Start the service (best effort) and wait for it to accept connections.
  sudo service postgresql start 2>/dev/null \
    || sudo systemctl start postgresql 2>/dev/null \
    || true
  if have pg_isready && ! pg_isready -q -t 10 2>/dev/null; then
    warn "Postgres is not accepting connections — skipping provisioning"
    return 0
  fi

  # Resolve the desired connection from .env's PG_URL.
  local pg_url
  pg_url=$(grep -E '^(export )?PG_URL=' "$envfile" | head -1 | sed -E 's/^(export )?PG_URL=//' || true)
  [ -n "$pg_url" ] || { warn "PG_URL not found in .env — skipping Postgres provisioning"; return 0; }
  pg_url="${pg_url%\"}"; pg_url="${pg_url#\"}"   # strip surrounding quotes

  # p_pass may be the .env placeholder; for an existing role we verify it over TCP
  # before trusting it, and fall back to prompting (see the existing-role branch).
  local p_user p_pass p_host p_port p_db
  IFS=$'\x1f' read -r p_user p_pass p_host p_port p_db < <(_pg_parse_url "$pg_url") || true

  # Default the role to the current OS user when .env still has the placeholder.
  local role="$p_user"
  if [ -z "$role" ] || [ "$role" = "user" ]; then
    role="${USER:-$(whoami)}"
  fi
  local host="${p_host:-localhost}" port="${p_port:-5432}"

  # Database name: reuse the real value already in .env. Otherwise (scaffolding
  # set no name, or the kit's "app" placeholder is still here) derive a per-project
  # default from the directory name — so two projects on one cluster don't collide
  # on a single "app" db — and let the developer confirm/override. This is the
  # fallback for SCAFFOLD.md's up-front interview, mirroring the copyright holder.
  local dbname="$p_db"
  if [ -z "$dbname" ] || [ "$dbname" = "app" ]; then
    local default_db input_db
    default_db="$(_pg_sanitize_dbname "$(basename "$PWD")")"
    read -r -p "  PostgreSQL database name [${default_db}]: " input_db || true
    dbname="$(_pg_sanitize_dbname "${input_db:-$default_db}")"
    log "Using database '${dbname}'"
  fi

  local resolved_pass="" have_creds=false

  # Can we act as the postgres superuser? Needed to inspect/create roles & dbs.
  log "Checking PostgreSQL superuser access (may prompt for sudo)"
  if sudo -u postgres psql -tAc 'SELECT 1' >/dev/null 2>&1; then
    local role_exists
    role_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${role}'" 2>/dev/null || true)

    if [ "$role_exists" != "1" ]; then
      # Brand-new role: generate a password, create with CREATEDB, record it.
      log "Creating PostgreSQL role '${role}'"
      resolved_pass="$(openssl rand -hex 16)"
      if sudo -u postgres psql -q -c \
          "CREATE USER ${role} WITH ENCRYPTED PASSWORD '${resolved_pass}' CREATEDB;" 2>/dev/null; then
        have_creds=true
      else
        warn "failed to create role '${role}' — skipping Postgres provisioning"
        return 0
      fi
    else
      # Existing role: reconcile credentials, testing the SAME way the app connects
      # (TCP to $host), so we never record creds that work only over the socket.
      log "PostgreSQL role '${role}' already exists — reconciling credentials"
      if psql "$(_pg_url "$role" '' "$host" "$port" postgres)" -tAc 'SELECT 1' >/dev/null 2>&1; then
        # Passwordless over TCP (trust auth): the app needs no password either.
        log "Role '${role}' connects over TCP without a password (trust auth)"
        have_creds=true                          # resolved_pass stays empty
      elif [ -n "$p_pass" ] && [ "$p_pass" != "password" ] \
             && PGPASSWORD="$p_pass" psql \
                  "$(_pg_url "$role" "$p_pass" "$host" "$port" postgres)" -tAc 'SELECT 1' >/dev/null 2>&1; then
        # A real password already in .env that works over TCP: keep it.
        resolved_pass="$p_pass"; have_creds=true
        log "Existing password from .env verified for role '${role}'"
      else
        # Prompt for the password the role already uses (may be shared with other
        # apps). A blank entry means "I have no password — generate one for me".
        local tries=0 input
        while [ "$tries" -lt 3 ]; do
          read -r -s -p "  PostgreSQL password for role '${role}' (blank to generate & set a new one): " input; echo
          [ -z "$input" ] && break
          if PGPASSWORD="$input" psql \
               "$(_pg_url "$role" "$input" "$host" "$port" postgres)" -tAc 'SELECT 1' >/dev/null 2>&1; then
            resolved_pass="$input"; have_creds=true
            log "Password verified for role '${role}'"
            break
          fi
          warn "could not connect with that password"
          tries=$((tries + 1))
        done

        # Last resort: no working password. The role likely uses peer auth (fine on
        # the local socket, unusable over TCP). Offer to set one -- with consent,
        # since this ALTERs a role that may be shared with other apps.
        if [ "$have_creds" != true ]; then
          warn "no TCP-usable password for role '${role}' (it likely uses peer auth on"
          warn "the local socket). The app connects over TCP and needs a password."
          warn "Setting one ALTERs the role; if it is shared with other apps that may break them."
          read -r -p "  Generate and set a new password on role '${role}'? [y/N]: " _consent
          if [[ "${_consent:-}" =~ ^[Yy] ]]; then
            resolved_pass="$(openssl rand -hex 16)"
            if sudo -u postgres psql -q -c \
                 "ALTER ROLE ${role} WITH ENCRYPTED PASSWORD '${resolved_pass}';" 2>/dev/null; then
              have_creds=true
              log "Set a new password on role '${role}'"
            else
              warn "failed to set password on role '${role}' — leaving .env unchanged"; return 0
            fi
          else
            warn "declined — leaving .env unchanged. Set PG_URL by hand or re-run to set a password."; return 0
          fi
        fi
      fi
    fi

    # Ensure the database exists, owned by the role.
    local db_exists
    db_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${dbname}'" 2>/dev/null || true)
    if [ "$db_exists" != "1" ]; then
      log "Creating database '${dbname}'"
      if sudo -u postgres createdb -O "${role}" "${dbname}" 2>/dev/null; then
        log "Database '${dbname}' created"
      else
        warn "failed to create database '${dbname}'"
      fi
    fi
  else
    warn "cannot act as the postgres superuser (sudo -u postgres failed)"
    warn "skipping role/db creation; will only test the credentials already in .env"
  fi

  # Persist resolved credentials into .env (only when we determined them here).
  if [ "$have_creds" = true ]; then
    local newurl; newurl="$(_pg_url "$role" "$resolved_pass" "$host" "$port" "$dbname")"
    _pg_set_env PG_URL "$newurl" "$envfile"
    chmod 600 "$envfile" 2>/dev/null || true
    pg_url="$newurl"
  fi

  # Final connection test against the real target database.
  if PGPASSWORD="$resolved_pass" psql "$pg_url" -tAc 'SELECT 1' >/dev/null 2>&1; then
    log "PostgreSQL connection OK (${role}@${host}:${port}/${dbname})"
  else
    warn "PostgreSQL connection test failed for ${role}@${host}:${port}/${dbname}"
    warn "check credentials in .env and that the database exists"
  fi
}
