#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# fix_vulns.sh: upgrade dependencies with known vulnerabilities, then triage the
# rest into tests/vuln_allowlist.json. Run it with `make fix_vulns`.
#
#   1. Run govulncheck and collect every reachable finding that has a fix.
#   2. Take the highest fixed version per module and `go get` it. A standard
#      library fix bumps the go directive instead.
#   3. `go mod tidy`, then scan again.
#   4. For each reachable finding with no fix, ask whether to allowlist it, and
#      record the reason given.
#
# Exits non-zero when a fixable finding survives the upgrade, or when a finding
# with no fix is left out of the allowlist. TestGovulncheck fails on both.
set -euo pipefail

cd "$(dirname "$0")/.."

ALLOWLIST="tests/vuln_allowlist.json"

if ! command -v jq >/dev/null; then
    echo "jq is required but not installed" >&2
    exit 1
fi

SCAN_JSON="$(mktemp)"
trap 'rm -f "$SCAN_JSON"' EXIT

_scan() {
    echo "==> Running govulncheck"
    local scan_err
    scan_err="$(mktemp)"
    if ! go tool govulncheck -format json ./... > "$SCAN_JSON" 2>"$scan_err" \
        || [[ ! -s "$SCAN_JSON" ]]; then
        echo "govulncheck failed:" >&2
        cat "$scan_err" >&2
        rm -f "$scan_err"
        exit 1
    fi
    rm -f "$scan_err"
}

# Prints the older of two versions.
_older() {
    printf '%s\n%s\n' "$1" "$2" | sort -V | head -1
}

# --- Phase 1: upgrade everything fixable ---------------------------------
_scan

mapfile -t UPGRADES < <(jq -r -f scripts/vuln_upgrades.jq "$SCAN_JSON" | sort -u)

if [[ ${#UPGRADES[@]} -eq 0 ]]; then
    echo "No fixable reachable vulnerabilities."
else
    declare -A MAX_FIX
    for entry in "${UPGRADES[@]}"; do
        IFS=$'\t' read -r mod fixed <<<"$entry"
        cur="${MAX_FIX[$mod]:-}"
        if [[ -z "$cur" ]] || [[ "$(_older "$cur" "$fixed")" == "$cur" ]]; then
            MAX_FIX[$mod]="$fixed"
        fi
    done

    # A standard library fix needs a local Go at least that new. GOTOOLCHAIN=local
    # reports the installed SDK, not a toolchain go.mod would download.
    if [[ -n "${MAX_FIX[stdlib]:-}" ]]; then
        required="${MAX_FIX[stdlib]#v}"
        current="$(GOTOOLCHAIN=local go version | awk '{print $3}' | sed 's/^go//')"
        if [[ "$(_older "$current" "$required")" != "$required" ]]; then
            echo "The standard library fix needs Go ${required}, and the installed Go is ${current}." >&2
            echo "Install Go ${required} from https://go.dev/dl/ and run this again." >&2
            exit 1
        fi
    fi

    echo "==> Applying upgrades"
    for mod in "${!MAX_FIX[@]}"; do
        fixed="${MAX_FIX[$mod]}"
        if [[ "$mod" == "stdlib" ]]; then
            echo "  stdlib -> go ${fixed#v} (go.mod directive)"
            go mod edit -go="${fixed#v}" -toolchain=none
        else
            echo "  $mod -> $fixed"
            if ! go get "${mod}@${fixed}"; then
                echo "  go get failed for ${mod}@${fixed}" >&2
            fi
        fi
    done

    echo "==> go mod tidy"
    go mod tidy
fi

# --- Phase 2: scan again and triage what is left -------------------------
_scan

mapfile -t REMAINING < <(jq -r -f scripts/vuln_remaining.jq "$SCAN_JSON" | sort -u)

declare -A SUMMARY
while IFS=$'\t' read -r id summary; do
    SUMMARY[$id]="$summary"
done < <(jq -r 'select(.osv != null) | "\(.osv.id)\t\(.osv.summary)"' "$SCAN_JSON")

declare -A IN_ALLOW
if [[ -f "$ALLOWLIST" ]]; then
    allowed_ids="$(jq -r '.allowlist[]?.id // empty' "$ALLOWLIST")"
    while read -r id; do
        [[ -n "$id" ]] && IN_ALLOW[$id]=1
    done <<<"$allowed_ids"
fi

declare -A SEEN
NEW_ENTRIES=()
HAS_FIXABLE_RESIDUE=0
DECLINED=0

for entry in "${REMAINING[@]}"; do
    IFS=$'\t' read -r id fixed <<<"$entry"
    [[ -n "${SEEN[$id]:-}" ]] && continue
    SEEN[$id]=1

    if [[ -n "$fixed" ]]; then
        echo "  $id is still fixable (fixed in $fixed); the upgrade did not take" >&2
        HAS_FIXABLE_RESIDUE=1
        continue
    fi
    [[ -n "${IN_ALLOW[$id]:-}" ]] && continue

    echo ""
    echo "  $id: ${SUMMARY[$id]:-(no summary)}"
    if ! read -r -p "  Append to $ALLOWLIST? [y/N] " ans; then
        echo "  stdin closed; stopping triage" >&2
        DECLINED=1
        break
    fi
    if [[ "$ans" =~ ^[Yy]$ ]]; then
        if ! read -r -p "  Reason: " reason; then
            echo "  stdin closed; stopping triage" >&2
            DECLINED=1
            break
        fi
        NEW_ENTRIES+=("$(jq -n --arg id "$id" --arg reason "$reason" '{id: $id, reason: $reason}')")
    else
        echo "  Skipped. TestGovulncheck keeps failing on $id"
        DECLINED=1
    fi
done

if [[ ${#NEW_ENTRIES[@]} -gt 0 ]]; then
    [[ -f "$ALLOWLIST" ]] || echo '{"allowlist": []}' > "$ALLOWLIST"
    tmp="$(mktemp)"
    new_json="[$(IFS=,; echo "${NEW_ENTRIES[*]}")]"
    jq --argjson new "$new_json" '.allowlist = ((.allowlist // []) + $new | unique_by(.id))' \
        "$ALLOWLIST" > "$tmp"
    mv "$tmp" "$ALLOWLIST"
    echo "Added ${#NEW_ENTRIES[@]} entry/entries to $ALLOWLIST"
fi

if [[ "$HAS_FIXABLE_RESIDUE" == "1" ]]; then
    echo "Some fixable findings remain. Investigate, then run this again." >&2
    exit 1
fi
if [[ "$DECLINED" == "1" ]]; then
    exit 1
fi

echo ""
echo "Done. Run 'go test ./tests/ -run TestGovulncheck' to verify."
