---
name: review-code
description : Performs a code review of the current changes. Use when code is complete and ready to commit, or when a user asks "review this code"
---

<!-- SPDX-License-Identifier: MIT -->

## Identifying changes to review
When reviewing code, start with the changes to be reviewed. If the user specifies a changeset or a specific file, constrain the review to that file or changeset. Otherwise, look for files that are locally modified or about to be committed:
1. Use git to check for staged changes: add them to the list of files to review. Do review source code (.go, .ts, .py, .sql). Don't review .md
2. Use git to check for whether there are unstaged changes. If there is source code that is not tracked, add it to the list to review, but remind the user it is not currently staged


## What to look for

Flag every violation. For rules that say "recommend" or "check for X", follow that specific instruction; for all others a numbered complaint is sufficient. Rules marked [cross-file] are applied once across all files after the per-file pass completes.

For each file in the diff, read the entire file's changes, then apply every rule below to that file before moving to the next. Apply general rules to all files. Apply language-specific rules only to files of that type.

### General: Clarity

1. Is the code concise?
1. Does the code use descriptive variable names? Check if a variable is named to affect control flow but doesn't have the intended impact. In refactors or parallelization efforts, check if original variables need a substring added.
1. Do functions have comments? For 50+ line functions, do they describe the "what" and the "why" of the function?
1. Do major sections of code have comments? If the code is complicated, does it have comments throughout, walking the reader through the flow?
1. Are magic numbers that represent business rules or domain thresholds (regulatory limits, rate caps, timeouts, age/size boundaries) named as constants? Implementation-detail literals (byte offsets, fixed field widths, array indices) are acceptable with an inline comment instead.
1. [cross-file] Does the code follow the same naming patterns used elsewhere? E.g. is it a "userId" and not an "id".
1. Do variable names express a complete thought? Flag names that read as partial phrases
1. Does the code prefer a named function or method over a closure (anonymous function / lambda)? A closure is acceptable only when accompanied by a comment justifying why a named function won't do (e.g. it must capture local state that can't be cleanly passed as a parameter). Flag any closure that lacks such a justifying comment.

### General: Correctness

1. Is the code free of performance-sensitive issues? Does it avoid O(n) database transactions? Is the math efficient?
1. Is the code free of required manual intervention, such as a database update?
1. Do function callers correctly handle and report error conditions and/or sentinel return values?
1. Does each independently-failable operation have its own idempotency guard (hint, flag, watermark)? When new code appends a second operation after the first operation's guard has already committed, a failure in the second operation is silently swallowed — the guard prevents any retry. Each operation that must complete reliably needs its own guard, even when logically related.
1. Do tests check both the happy path and any failure paths? Do tests check cached data in addition to a fall-through fetch case?
1. Do tests avoid relying on special date values to protect against deleting test data? (Example: a test that sets dates to 2027 and cleans up all dates >= 2027 will fail once that year passes.)

### General: Design

1. If the code adds to a function longer than 300 lines, is there a todo.md item to address it?
1. Are the architectural tradeoffs described in the comments or in a secondary file?
1. Was a test added as part of this change? If not and one should be added, recommend (in 2-3 sentences) what the test should do.
1. [cross-file] Does the code reuse existing functions rather than duplicating similar logic that exists elsewhere?
1. Does the code keep concerns within the correct file? E.g. model code should not appear in a repository file.
1. Is production code free of changes that exist solely to accommodate testing?

### Language: JavaScript / React
Apply only when reviewing *.ts, *.tsx

1. If the code uses `useMemo`, does it include a comment explaining why inline or functional calculation isn't adequate and what the performance cost/sensitivity is? A single well-justified `useMemo` is acceptable; flag any without justification.
1. Is `useMemo` usage free of chains (two or more `useMemo` calls where one depends on another)? Chained `useMemo` is never acceptable regardless of justification — flag it and recommend restructuring.


### Language: Go
Apply only when reviewing *.go files

1. Do new API endpoints include appropriate swagger documentation describing what the endpoint does?

### Language: Scripting (Bash / Python)
Apply only when reviewing *.sh, *.py, *.pl files

1. Are embedded scripts kept short (under 5 lines, no special libraries)? If longer or using special libraries, are they written as separate files so they can be discovered by suffix?
1. Do script command invocations check return values?
1. Does the script redirect STDERR to /dev/null? Is it dropping errors it could report?

## How to report the problems
1. Number each complaint incrementally so that it can be addressed individually.
1. Each complaint should have source code file, line number, a brief description of the problem, with a suggested fix.
1. If a file has no violations, omit it from the report entirely
1. Do not emit items that conclude "no isse", "this is fine", "safe", "acceptable". 
