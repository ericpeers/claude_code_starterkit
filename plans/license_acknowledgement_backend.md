# License Acknowledgement Pipeline (Backend)

A portable pattern for tracking, gating, and serving third-party dependency
licenses. It has **three legs**, all built around a single license-scanner tool
and one generated artifact, `LICENSES.TXT`.

| Leg | Where | What it does |
|---|---|---|
| **Generate** | `bin/gen-notices` + Makefile `build` target | Scanner copies every dependency's LICENSE file into a scratch dir, then concatenates them into `LICENSES.TXT` with a per-module header |
| **Serve** | Embedded into the binary → handler at `GET /v1/licenses` | The artifact is compiled *into* the binary and served as `text/plain` |
| **Test / gate** | `tests/license_test.go` | Scanner report → CSV → every dependency's SPDX identifier must be in an allowlist; GPL / unknown licenses fail the test |

## Key design decisions

- **One source of truth, two consumers.** The license scanner drives both the
  human-readable `LICENSES.TXT` (generate leg) and the machine-checked allowlist
  gate (test leg). They cannot drift because they read the same dependency graph.
- **The artifact is embedded, not read at runtime.** Compiling the notices into
  the binary means the served text always matches the shipped binary — no
  missing-file or stale-file risk in production.
- **Allowlist, not denylist.** The policy lists what is *approved* (MIT,
  Apache-2.0, BSD, ISC, MPL-2.0, etc.). Anything unrecognized — including any
  GPL variant — fails closed.
- **The gate is cached on the lockfile hash.** The (slow) scanner subprocess is
  skipped unless dependencies change, but the policy check re-runs every time, so
  editing the allowlist takes effect immediately.
- **Build wires generation into `make build`** so `LICENSES.TXT` is regenerated
  on every build, and a dirty working tree after build fails the `deploy`
  pre-flight gate.

---

## Implementation plan

Written language-agnostically, with the Go-specific commands called out. The
reference implementation uses Google's [`go-licenses`](https://github.com/google/go-licenses).

### Phase 0 — Pick the license scanner for the target ecosystem

The whole pattern hinges on one tool that can (a) emit a machine-readable
per-dependency SPDX report and (b) export each dependency's full license text.
Choose the ecosystem equivalent:

- **Go:** `go install github.com/google/go-licenses@latest`
- **Node:** `license-checker` / `license-checker-rseidelsohn` (`--json`, `--out`)
- **Rust:** `cargo-about` (template-driven notice file) + `cargo-deny` (the gate)
- **Python:** `pip-licenses` (`--format=csv`, `--with-license-file`)
- **Java:** Maven `license-maven-plugin` / Gradle `com.github.jk1.dependency-license-report`

Every step below maps onto whichever tool you pick — the structure is identical.

### Phase 1 — Define the policy (the allowlist)

Create an **allowlist of approved SPDX identifiers** in the test file. Start with
`MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, Unlicense, CC0-1.0`. Add
weak-copyleft (`MPL-2.0`) only with a comment explaining the obligation and why
it is acceptable (MPL-2.0 imposes source-disclosure obligations only on
modifications to the MPL'd files themselves, not the application as a whole —
acceptable as long as you do not modify the upstream MPL'd sources). **Never add
GPL / AGPL / LGPL** without legal sign-off; the point of fail-closed is that they
surface as violations automatically.

```go
// permissiveLicenses is the allowlist of SPDX identifiers acceptable for
// commercial use. Add an entry only after verifying the license terms are
// compatible with this application.
var permissiveLicenses = map[string]bool{
    "MIT":          true,
    "Apache-2.0":   true,
    "BSD-2-Clause": true,
    "BSD-3-Clause": true,
    "ISC":          true,
    "MPL-2.0":      true, // file-scoped copyleft; safe while we don't modify the upstream source
    "Unlicense":    true,
    "CC0-1.0":      true,
}
```

### Phase 2 — The test gate (CI enforcement)

Write a test that:

1. Locates the scanner binary; **skip (don't fail) if it is not installed** so a
   fresh checkout without the tool still passes locally — CI installs it explicitly.
2. Runs the scanner's *report* mode → parses the per-module / SPDX output.
3. **Excludes first-party modules** by module-path prefix.
4. Flags any dependency whose SPDX identifier is not in the allowlist; fail with
   a message listing each `module → license` and the remediation ("verify, then
   add to the allowlist or replace the dependency").
5. **Cache the scanner output** keyed on a hash of the lockfile
   (`go.sum` / `package-lock.json` / `Cargo.lock`) plus the binary's mtime, so the
   slow subprocess only runs when dependencies change. The allowlist check itself
   always re-runs against the cache, so policy edits take effect immediately.

Scope the report to the main package (`.`) rather than `./...` — the binary's
import graph already covers every third-party module, so test-only packages add
no new license modules while loading them adds wall time.

### Phase 3 — The generate leg (notice artifact)

Write a `bin/gen-notices` script that:

1. Runs the scanner's *save / export* mode into a scratch dir (`build/licenses/`).
2. Concatenates every license file into a single `LICENSES.TXT`, each preceded by
   a `=== module-name ===` header, sorted deterministically.
3. Is idempotent and `set -euo pipefail`-strict — fail loudly if the tool is
   missing, with the install command in the error message.
4. Add `build/licenses/` to `.gitignore`; commit `LICENSES.TXT` itself (it is an
   artifact reviewers want to diff).

```bash
#!/usr/bin/env bash
# bin/gen-notices — Generate LICENSES.TXT from third-party dependency licenses.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LICENSES_DIR="$ROOT/build/licenses"
OUTPUT="$ROOT/LICENSES.TXT"

# ...resolve the scanner binary, exit 1 with install hint if missing...

rm -rf "$LICENSES_DIR"
go-licenses save ./... --save_path="$LICENSES_DIR" --ignore <your-module-path>

{
    echo "THIRD-PARTY SOFTWARE NOTICES"
    echo "This file contains license notices for open source software used in this project."
    echo ""
    while IFS= read -r -d '' file; do
        module="${file#"$LICENSES_DIR"/}"
        echo "================================================================"
        echo "$module"
        echo "================================================================"
        cat "$file"
        echo ""
    done < <(find "$LICENSES_DIR" -type f -print0 | sort -z)
} > "$OUTPUT"

echo "Generated $OUTPUT ($(wc -l < "$OUTPUT") lines)"
```

### Phase 4 — Wire into build & deploy

In the Makefile (or build script):

- `build:` runs `gen-notices` *before* compiling, so the artifact is always current.
- `deploy:` (pre-flight) asserts a clean working tree, runs `test` and `build`,
  then re-asserts clean — a `LICENSES.TXT` diff after build means someone changed
  dependencies without regenerating, and deploy aborts.

```makefile
build:
	bin/gen-notices
	go build -o app .

deploy:
	@test -z "$$(git status --porcelain)" || \
		(echo "ERROR: uncommitted changes — clean working tree required to deploy"; exit 1)
	@$(MAKE) test
	@$(MAKE) build
	@test -z "$$(git status --porcelain)" || \
		(echo "ERROR: git no longer clean after building — regenerate LICENSES.TXT"; exit 1)
```

### Phase 5 — The serve leg (endpoint)

- Embed the artifact into the binary at compile time (Go `//go:embed`; Node
  bundle / `fs` at module init; Rust `include_str!`). Embedding guarantees the
  served text matches the shipped binary.
- Expose a read-only `GET /licenses` (or `/v1/licenses`) returning the bytes as
  `text/plain; charset=utf-8`.
- Document it in your API docs (annotate for OpenAPI / Swagger).
- Test the endpoint returns 200 plus the embedded content.

```go
//go:embed LICENSES.TXT
var licensesText []byte

// GetLicenses godoc
// @Summary      Third-party software licenses
// @Description  Returns the complete license text for all open source dependencies.
// @Tags         info
// @Produce      plain
// @Success      200  {string}  string  "License notices"
// @Router       /licenses [get]
func (h *InfoHandler) GetLicenses(c *gin.Context) {
    c.Data(http.StatusOK, "text/plain; charset=utf-8", h.licensesText)
}
```

---

## What a repo gets from this

- **Fail-closed licensing:** a GPL dependency cannot reach production — CI
  rejects it the moment it enters the lockfile.
- **No drift:** the same scanner feeds both the served notices and the gate.
- **Self-documenting binary:** `GET /licenses` always reflects exactly what shipped.
- **Cheap in CI:** lockfile-keyed caching keeps the gate near-free on unchanged
  dependency sets.
