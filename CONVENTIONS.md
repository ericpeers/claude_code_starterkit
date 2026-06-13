# Engineering Conventions

Language-agnostic working discipline shared by every stack in this kit. Each
`CLAUDE_<stack>.md` references this file; when scaffolding a new repo, the
relevant points are inlined into that repo's `CLAUDE.md` so it stands alone.

## Bug fixes: write the failing test first

When fixing a reported bug or code-review finding, follow this order strictly:

1. **Write the test first.** Encode the expected behavior as a concrete
   assertion against the exact scenario the report describes (specific inputs,
   counts, edge case, dates).
2. **Run it against the current (unfixed) code.** Confirm it fails, AND that it
   fails for the reason the report describes — not some unrelated setup problem.
   Read the error output before proceeding.
3. **Apply the fix.**
4. **Re-run the test.** It must now pass. Run the surrounding suite too to catch
   regressions in adjacent code.

A test written *after* the fix only proves "this code path produces this value";
it does not prove the test would have caught the original bug. The red-then-green
cycle is the only way to confirm the test is load-bearing.

Skip only for purely cosmetic changes (typo, log wording) or when reproduction
cost clearly exceeds the protection value (e.g. a one-time data migration).

## Plan execution discipline

When executing an approved plan:

* **Never silently deviate.** If implementation reveals a problem the plan didn't
  anticipate, stop and say what broke and why — don't quietly change approach.
* **Treat plan-specified values as constraints, not suggestions.** If a value was
  chosen for a stated reason, solve obstacles around the constraint rather than
  changing it.
* **Fix the root cause, not the symptom.** When a test fails, trace *why* before
  changing inputs. Changing a carefully chosen value to make a test pass is a red
  flag that you're fixing the wrong layer.

## Confirm before destructive or DDL operations

Always ask before running `DROP`, `CREATE`, `ALTER`, `TRUNCATE`, or any
data-mutating command against a live/shared database — even when it follows from
an approved plan. The same applies to destructive filesystem or infrastructure
operations (deleting resources, overwriting files you didn't create).

## Verify before claiming

Before asserting anything about current state — "X is removed", "no longer does
Y", "now returns Z" — Read the actual file. Do not rely on a grep hit or an
earlier read; both go stale. Every factual claim, including closing summary
notes, must be verified with a tool *before* you state it. Checking is your job,
not a question to hand back to the user.

## Code review: no positive commentary

In code reviews, report only actionable problems. Drop any item that concludes
"good", "fine", "OK", "safe", or "acceptable" — if there is no fix to make, leave
it out entirely.

## Prefer named functions over closures

Prefer a named function or method over a closure (anonymous function / lambda).
A closure is acceptable only with a comment justifying why a named function won't
do (e.g. it must capture local state that can't be cleanly passed as an argument).

## Diagnostic throwaway tests for hard bugs

When investigating a hard-to-reproduce bug, write a dedicated diagnostic test
file (e.g. `tests/debug_<topic>_test.*`) with a `THROWAWAY` header that reproduces
the exact scenario. Use it to confirm the root cause, then delete it once the
real, permanent test is in place.

## Trace concrete inputs through every pipeline stage

When a multi-stage pipeline (resolver → validator → normalizer → expander, or
similar) produces wrong output, pick one specific failing input and write down
what happens at *every* stage. The bug is often in the interaction between
stages, not in the stage that "looks wrong." Don't just reason about the
suspicious-looking stage in isolation.

## Debugging discipline

* Include entity identifiers (IDs, names, keys) in debug logging so findings
  can be cross-referenced.
* Cross-reference each finding against earlier evidence; don't build a narrative
  ahead of the evidence.
* Test the user's stated theory directly rather than assuming it's right or wrong.

## Module boundary / ownership isolation

A module should only *mutate* the data (tables, files, resources) it owns. If you
need data owned by another module, call that module's API rather than reaching
in. Read-only joins/lookups across boundaries are fine; cross-boundary writes are
not. (See the Go stack's `TestRepositoryTableOwnership` for a way to enforce this
mechanically.)

## Large result sets: stream, don't accumulate

When a query or transform could yield very large output (bulk export, full-table
scan), stream row-by-row via a callback instead of accumulating into an in-memory
slice/list. Accumulation causes O(n) memory and GC pressure that dwarfs other
costs. Pre-fetch small sparse side-tables into a map and merge in code rather
than paying a per-row JOIN across millions of rows.

## Workflow

* Don't suggest committing after completing work — the author tests and iterates,
  then commits when ready.
* After finishing, state plainly that the work is done. Don't tack on a follow-up
  technical question based on incomplete context.
