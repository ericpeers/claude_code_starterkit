# Claude Code Starter Kit

A reusable set of [Claude Code](https://claude.com/claude-code) conventions,
generic quality/vulnerability tests, dev-setup scripts, git hooks, and editor
settings for spinning up new projects with good habits baked in from commit zero.

The kit ships four stacks plus a shared layer:

| Stack | Language / framework | Folder |
|---|---|---|
| Go | Go backend (HTTP API + Postgres) | [`go/`](go/) |
| Python | Python 3.12 services / scrapers | [`python/`](python/) |
| React | React + TypeScript + Vite | [`react/`](react/) |
| Infra | AWS CDK (TypeScript) | [`infra/`](infra/) |

Cross-cutting pieces live in [`shared/`](shared/) and [`CONVENTIONS.md`](CONVENTIONS.md).

## How to use it

The kit is **agent-driven**. Clone it next to a new (empty) repo, then prompt a
fresh Claude Code agent:

> using ../claude_code_starterkit, construct a go scaffold in the current directory

The agent reads [`SCAFFOLD.md`](SCAFFOLD.md) — the deterministic, per-stack recipe
— and assembles the new project: `CLAUDE.md` (with the relevant conventions
inlined), generic tests, `setup_dev.sh`, `.env_sample`, `.gitignore`, a
`.githooks/pre-commit` test gate, and a `.claude/` directory containing
`settings.json` and the `review-code` skill.

Then in the new repo:

```bash
./setup_dev.sh        # installs toolchain, wires git hooks, records your copyright holder
```

> **License headers:** every file the kit supplies carries
> `SPDX-License-Identifier: MIT` and is owned by you under the top-level
> [`LICENSE`](LICENSE) — no personal name is stamped into individual files. The
> infra stack also ships a header gate (`copyright_checks.test.ts`) that exempts
> SPDX-tagged files and requires *your own* source to carry a
> `Copyright (c) <year> <holder>` header, where the holder is whatever
> `setup_dev.sh` writes into `.copyright-holder`. A fresh scaffold is green; the
> gate only bites once you add your own un-headered files.

## What's generic vs. yours

Everything here was distilled from real projects and **scrubbed of
business-domain logic**. The tests that ship are the ones with universal value:
dependency vulnerability gates, license allowlists, dead-code detection, shell
lint, schema-drift and module-ownership templates. Domain tests are yours to add.

## License

[MIT](LICENSE) — Copyright (c) 2026 Eric Peers.
