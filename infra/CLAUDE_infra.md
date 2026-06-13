# CLAUDE.md — Infra stack (AWS CDK / TypeScript)

> Working discipline that applies to every stack lives in `CONVENTIONS.md`
> (failing-test-first, plan execution, verify-before-claiming, confirm before
> destructive ops, etc.). The scaffold inlines the relevant parts into this file.
> This document adds infrastructure-specific guidance.

## Layout

* `bin/app.ts` — CDK app entry (wires stacks).
* `lib/` — stack and construct definitions.
* `scripts/` — operational shell scripts (deploy, tunnels, db access). `lib.sh`
  holds shared helpers and is sourced, never run directly.
* `tests/` — the quality gates (lint, copyright, security) and the cdk-nag
  template.
* `cdk.json` — note `outputsFile: cdk_outputs.json`; scripts read deploy outputs
  from that file rather than calling the CloudFormation API.

## Stack ordering

Define resources in dependency order so references resolve cleanly, typically:
VPC → security groups → IAM → storage (S3) → database (RDS) → compute (EC2/Lambda)
→ CDN (CloudFront). Keep a single stack readable top-to-bottom in that order.

## Deployment

* `npx cdk deploy` writes outputs to `cdk_outputs.json`. Operational scripts
  consume that file — keep output keys stable.
* Use a clean-git-tree pre-flight gate before deploying (`check_git_status` in
  `scripts/lib.sh`).

## AWS gotchas (hard-won)

* **Pass `env` (account + region) explicitly** to stacks that look up
  region-specific values (e.g. CloudFront prefix lists), or those lookups won't
  resolve.
* **ASCII-only strings** in user-data / metadata — non-ASCII can break cloud-init.
* **IMDSv2**: require tokens; set the unique-template-name feature flag.
* **EC2 replacement**: changing some properties replaces the instance. Know which
  changes are in-place vs. replacing before you deploy to a live host.
* **CloudFormation** can get stuck on a failed update; know your recovery path
  (continue-rollback, or delete+redeploy) before you need it.
* Every `aws` CLI call in a script must pass `--region` (enforced by
  `tests/lint_checks.test.ts`), except credential bootstrap in `setup_dev.sh`.

## cdk-nag: suppress consciously, with a deadline

Run `AwsSolutionsChecks` over the app in tests. Every finding is either fixed or
suppressed **with a written justification**. Suppressions carry a
`SUPPRESSION_REVIEW_BY` date — when it passes, the suite goes red until each
tradeoff is re-evaluated and the date is pushed forward (or the suppression
removed). See `tests/cdk_nag_suppressions.test.ts.template` to activate this.

## Shell-script quality

`tests/lint_checks.test.ts` enforces:

* `shellcheck -x` clean across all scripts.
* `set -euo pipefail` at the top of every script.
* Required env vars guarded with `${VAR:?message}` so a missing var fails fast.

Keep embedded inline scripting short; longer logic belongs in a discoverable
`scripts/*.sh` file.

## License / copyright header gate

`tests/copyright_checks.test.ts` enforces per-file licensing on `.ts` and
`scripts/*.sh` files:

* Files carrying `SPDX-License-Identifier:` are **exempt** — that's how the kit's
  own MIT boilerplate passes without a personal name.
* Every other (your own) file must carry a current-year
  `Copyright (c) <year> <holder>` header, where the holder comes from the
  committed `.copyright-holder` file that `./setup_dev.sh` writes.

A fresh scaffold is green (everything shipped is SPDX-exempt); the gate starts
enforcing once you add your own source without an SPDX tag or copyright header.
Your ownership of the project as a whole lives once in `LICENSE`.

## Environment notes

* `.stage.env` holds remote-host settings (`STAGE_*`, tunnel token); it is
  gitignored.
* **WSL2 DNS:** WSL2 can serve stale `NXDOMAIN` from its default resolver
  (`10.255.255.254`). If AWS endpoints intermittently fail to resolve, point
  `/etc/resolv.conf` at `1.1.1.1` and persist it with
  `[network] generateResolvConf = false` in `/etc/wsl.conf`.
