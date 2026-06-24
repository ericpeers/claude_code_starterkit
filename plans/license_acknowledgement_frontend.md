# License Acknowledgement Pipeline (Frontend)

A portable pattern for tracking, gating, and surfacing third-party dependency
licenses in a web app. It is the frontend twin of
[`license_acknowledgement_backend.md`](./license_acknowledgement_backend.md):
same **three legs**, same single-scanner / single-artifact discipline, adapted to
a Node + bundler (Vite/React) world where the app *is* static assets rather than a
binary.

| Leg | Where | What it does |
|---|---|---|
| **Generate** | `scripts/generate-notices.mjs`, wired into the `build` npm script | Scanner reads every production dependency's LICENSE file off disk and concatenates them into `public/LICENSE.txt` with a per-package header |
| **Serve** | The artifact ships as a static asset → fetched at `/LICENSE.txt` | The bundler copies `public/LICENSE.txt` into the build output; the app fetches it at runtime and renders it |
| **Test / gate** | `src/__tests__/licenses.test.ts` | Scanner report → every dependency's SPDX identifier must be in an allowlist; GPL / unknown licenses fail the test |

## Key design decisions

- **One source of truth, two consumers.** The license scanner
  (`license-checker-rseidelsohn`) drives both the human-readable `LICENSE.txt`
  (generate leg) and the machine-checked allowlist gate (test leg). They read the
  same `node_modules` graph, so they cannot drift.
- **The artifact is a build output, not hand-maintained.** It is regenerated on
  every `npm run build` from what is actually installed, so it can never fall out
  of sync with the dependency tree.
- **Static asset, not an endpoint.** A web app is already a pile of static files;
  the cheapest correct surface is `public/LICENSE.txt`, served by the same host as
  the bundle. (The backend, having no static host, serves *its* deps from a
  `GET /v1/licenses` route — see the backend plan. The two are merged at the
  *link* layer, never the file layer.)
- **Allowlist, not denylist.** The policy lists what is *approved*. Anything
  unrecognized — including any GPL variant — fails closed.
- **Two scan scopes, one tool.** The gate scans *everything* installed
  (build-time tools included, so a transitively-pulled GPL tool surfaces); the
  generator scans `production: true` only, so the shipped notice file lists just
  what reaches users.
- **One human surface aggregates every component.** A single "Acknowledgements"
  page shows a **Web** section (this app's `LICENSE.txt`) and an **API** section
  (a link to the backend's `/licenses` endpoint), so a user or auditor sees the
  whole system's third-party usage in one place.

---

## Implementation plan

Written for a Node/Vite/React app; the reference scanner is
[`license-checker-rseidelsohn`](https://www.npmjs.com/package/license-checker-rseidelsohn).
The structure maps onto any bundler (Next.js `public/`, CRA `public/`) and any
SPA framework.

### Phase 0 — Pick the license scanner

The pattern hinges on one tool that can (a) emit a machine-readable per-package
SPDX report and (b) point at each package's full license file on disk.

- **Node:** `license-checker-rseidelsohn` (programmatic `init()` API, `production`
  and `excludePrivatePackages` options) — used here.
- Alternatives: `license-checker`, `oss-attribution-generator`,
  `generate-license-file`.

Install as a dev dependency: `npm i -D license-checker-rseidelsohn`.

### Phase 1 — Define the policy (the allowlist)

Create an **allowlist of approved SPDX identifiers** as a `Set` in the test file.
Annotate every non-obvious entry with *why* it is acceptable — the comments are
the audit trail.

```ts
const ALLOWED_LICENSES = new Set([
  'MIT',
  'MIT-0',        // MIT without attribution requirement
  'Apache-2.0',
  'ISC',
  'BSD-2-Clause',
  'BSD-3-Clause',
  '0BSD',
  'CC0-1.0',
  'CC-BY-3.0',    // data/content packages (spdx-*)
  'CC-BY-4.0',    // data/content packages (caniuse-lite)
  'MPL-2.0',      // weak copyleft; build-time tools only, not distributed in app
  'Unlicense',
  'Python-2.0',
  'BlueOak-1.0.0',
  'OFL-1.1',      // SIL Open Font License — bundled fonts
]);
```

The npm ecosystem pulls in more license variety than most — expect `CC-BY-*` from
SPDX/caniuse data packages and `OFL-1.1` from any self-hosted fonts. **Never add
GPL / AGPL / LGPL** without legal sign-off; fail-closed exists so they surface as
violations automatically.

### Phase 2 — The test gate (CI enforcement)

Write a Jest/Vitest test that:

1. Scans installed packages via the scanner's programmatic API
   (`excludePrivatePackages: true` to drop your own workspace packages).
2. **Tokenizes compound SPDX expressions** — split `(MIT OR Apache-2.0)`, `AND`
   chains, and comma lists into individual identifiers before checking. npm
   declares far more dual-licensed packages than other ecosystems, so this step
   is mandatory, not optional.
3. Flags any package with a token not in the allowlist; fail with a message
   listing each `package → license` so the remediation is obvious (verify, then
   add to the allowlist or replace the dependency).

```ts
function toTokens(licenses: string | string[] | undefined): string[] {
  if (!licenses) return ['UNKNOWN'];
  const raw = Array.isArray(licenses) ? licenses : [licenses];
  return raw.flatMap((l) =>
    l.replace(/[()]/g, '')
     .split(/\s+(?:OR|AND)\s+|,\s*/)
     .map((s) => s.trim())
     .filter(Boolean),
  );
}

it('all packages use commercially permissive licenses', async () => {
  const packages = await scanPackages(); // init({ excludePrivatePackages: true }, …)
  const violations = Object.entries(packages)
    .filter(([, info]) => toTokens(info.licenses).some((t) => !ALLOWED_LICENSES.has(t)))
    .map(([name, info]) => `${name}: ${JSON.stringify(info.licenses)}`);
  expect(violations).toEqual([]);
});
```

**Policy choice — `some` vs `every`.** The example above passes a package if
*any* token is allowed (the liberal reading of `MIT OR GPL-3.0` — you may take the
MIT terms). If your legal posture requires that *every* disjunct be acceptable,
invert to `.every()`. Decide deliberately and comment it.

Because the gate is a normal unit test, it runs in CI for free — no separate job.

### Phase 3 — The generate leg (notice artifact)

Write `scripts/generate-notices.mjs` that:

1. Scans with `production: true, excludePrivatePackages: true` — only deps that
   ship to users.
2. Reads each package's `licenseFile` off disk and concatenates into
   `public/LICENSE.txt`, each block headed by `Package: name@version`,
   `Repository:`, and the `License:` SPDX id, sorted deterministically.
3. **Guards against junk text.** The scanner sometimes resolves `package.json` or
   `README.md` as the "license file" when no dedicated `LICENSE` exists — detect
   that and substitute a placeholder instead of dumping a whole README.
4. Exits non-zero on scan failure so a broken scan breaks the build.

```js
function licenseText(licenseFile) {
  if (!licenseFile) return '[No license file found]';
  const lower = licenseFile.toLowerCase();
  if (lower.endsWith('package.json') || lower.endsWith('readme.md') || lower.endsWith('readme')) {
    return '[License text not separately available — see SPDX identifier above]';
  }
  try { return readFileSync(licenseFile, 'utf8').trim(); }
  catch { return '[License file not readable]'; }
}

init({ start: root, production: true, excludePrivatePackages: true }, (err, packages) => {
  if (err) { console.error('License scan failed:', err.message); process.exit(1); }
  const lines = ['THIRD-PARTY SOFTWARE NOTICES AND INFORMATION', '='.repeat(45), ''];
  for (const [nameVersion, info] of Object.entries(packages).sort()) {
    const license = Array.isArray(info.licenses) ? info.licenses.join(', ') : (info.licenses ?? 'UNKNOWN');
    lines.push('='.repeat(80), '', `Package:    ${nameVersion}`);
    if (info.repository) lines.push(`Repository: ${info.repository}`);
    lines.push(`License:    ${license}`, '', licenseText(info.licenseFile), '');
  }
  writeFileSync(join(root, 'public', 'LICENSE.txt'), lines.join('\n'), 'utf8');
});
```

`public/LICENSE.txt` may be committed (reviewers like to diff it) or gitignored
and regenerated in CI — pick one and be consistent so the build-cleanliness check
in Phase 4 is meaningful.

### Phase 4 — Wire into the build

Put generation *before* compile in the build script so the artifact is always
current and a scan failure aborts the build:

```json
{
  "scripts": {
    "generate:notices": "node scripts/generate-notices.mjs",
    "build": "node scripts/generate-notices.mjs && tsc -b && vite build"
  }
}
```

If `public/LICENSE.txt` is committed, add a CI step (or `predeploy` check) that
fails on a dirty working tree after `npm run build` — a `LICENSE.txt` diff means
someone changed dependencies without regenerating. (Mirror of the backend plan's
`deploy` pre-flight gate.)

### Phase 5 — The serve leg (static asset + UI surface)

**Serving.** Anything under `public/` is copied verbatim into the build output by
Vite (likewise CRA/Next `public/`), so the artifact is reachable at `/LICENSE.txt`
on the same origin as the app. No endpoint, no server code.

**The UI surface.** Build one "Acknowledgements" page that aggregates *every*
component's notices by linking to each — the web app's own file and the backend's
endpoint:

- A tab or route (e.g. `/about` with an **Acknowledgements** tab) that is
  **deep-linkable** via a query param (`?tab=acknowledgements`) so other surfaces
  (a PDF report, a footer, settings) can point straight at it.
- A **Web** section that lazily fetches `/LICENSE.txt` (only when the tab is
  active; cache with `staleTime: Infinity`) and renders it in a scrollable
  `<pre>`, with explicit loading and error states.
- An **API** section linking out to the backend's `${apiBase}/licenses` endpoint
  (`target="_blank" rel="noreferrer"`).

```tsx
const { data: notices, isError, isLoading } = useQuery({
  queryKey: ['frontend-licenses'],
  queryFn: async () => {
    const r = await fetch('/LICENSE.txt');
    if (!r.ok) throw new Error('fetch failed');
    return r.text();
  },
  staleTime: Infinity,
  enabled: tab === 'acknowledgements', // don't fetch 100s of KB until viewed
});

// API section:
<a href={`${apiBase}/licenses`} target="_blank" rel="noreferrer">
  View API acknowledgements (third party licenses)
</a>
```

Test the page covers: loading state, fetch-failure error state, and that rendered
content includes a known package (e.g. `Package: react@…`) and that the API link
href contains the backend `/licenses` path.

---

## What a repo gets from this

- **Fail-closed licensing:** a GPL dependency cannot reach production — CI rejects
  it the moment it enters `package-lock.json`.
- **No drift:** the same scanner feeds both the served notices and the gate, and
  the artifact is regenerated on every build.
- **One surface for the whole system:** the Acknowledgements page links Web *and*
  API notices, so compliance review is a single click and other surfaces deep-link
  into it.
- **Cheap to serve:** notices ship as a static asset, lazily fetched only when a
  user opens the tab — zero server code, zero runtime cost until viewed.
