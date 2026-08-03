---
name: review-code
description : Performs a code review of the current changes. Use when code is complete and ready to commit, or when a user asks "review this code"
---

<!-- SPDX-License-Identifier: MIT -->

## Identifying changes to review
When reviewing code, start with the changes to be reviewed. If the user specifies a changeset or a specific file, constrain the review to that file or changeset. Otherwise, look for files that are locally modified or about to be committed:
1. Use git to check for staged changes: add them to the list of files to review. Do review source code (.go, .ts, .py, .sql). Don't review .md
2. Use git to check for whether there are unstaged changes. If there is source code that is not tracked, add it to the list to review, but remind the user it is not currently staged


## Review by reading first; linting second; specific tests only to confirm a finding
Do not open the review by running test suites and end to end tests. 

1) Read the code first. Reading files outside the diff is always fine and is expected — the [cross-file] rules require it.
2) Run fast linters and static checks as needed
3) Once a finding exists, if you need to run a narrowly scoped command, that takes no more than a few seconds, then run it. If it disproves the finding, drop the finding rather than reporting it hedged. If confirming would take a broad or slow run, ask for permission to run the larger test. 


## What to look for

Flag every violation. For rules that say "recommend" or "check for X", follow that specific instruction; for all others a numbered complaint is sufficient. Rules marked [cross-file] are applied once across all files after the per-file pass completes.

For each file in the diff, read the entire file's changes, then apply every rule below to that file before moving to the next. Apply general rules to all files. Apply language-specific rules only to files of that type.

### General: Clarity

1. Is the code concise?
1. Does the code use descriptive variable names? Check if a variable is named to affect control flow but doesn't have the intended impact. In refactors or parallelization efforts, check if original variables need a substring added.
1. Do functions have comments? For 50+ line functions, do they describe the "what" and the "why" of the function in no more than 3-4 lines?
1. Do major sections of code have comments? If the code is complicated, does it have 1-2 line comments every 10-20 lines, walking the reader through the flow?
1. Are magic numbers that represent business rules or domain thresholds (regulatory limits, rate caps, timeouts, age/size boundaries) named as constants? Implementation-detail literals (byte offsets, fixed field widths, array indices) are acceptable with an inline comment instead.
1. [cross-file] Does the code follow the same naming patterns used elsewhere? E.g. is it a "userId" and not an "id".
1. Do variable names express a complete thought? Flag names that read as partial phrases
1. Does the code prefer a named function or method over a closure (anonymous function / lambda)? Flag any closure without a comment justifying why a named or helper function won't do — e.g. it must capture local state that can't be cleanly passed as a parameter.
1. Is the code free of continuation-based programming? Flag any pattern that passes a function to be partially evaluated and then returns data to a new function (continuation-passing style). Prefer straight-line code that computes a value and returns it directly over threading control through passed-in continuations.
1. Are variable names at least 3 characters? Flag one- and two-character names. Exempt: loop indices `i`, `j`, `k`; `_`; and the language's established short idioms.

### General: Comments

1. Is each comment proportional to the code it documents? Flag as too long: more than 5 comment lines on a single constant, variable, or struct field; more than 15 on a function or type; more than 20 at file or package level. Also flag any reviewed file where total comment lines outnumber code lines. Quote the count and the declaration, and say which sentences to cut.
1. If a single declaration needs a long explanation, that is a design smell rather than good documentation — the declaration is usually carrying too much. Flag it and recommend one of two fixes: simplify the design, or move the reasoning to a design doc and leave a one-line comment pointing at it.
1. Does the comment state the decision instead of arguing for it? Flag persuasive or defensive prose: pre-empting objections nobody raised, rebutting alternatives nobody proposed, restating a point for emphasis, or rhetorical framing ("this is load-bearing, not a tuning constant"). One or two sentences of "why" is the budget.
1. Are comments free of ALL-CAPS words used for emphasis? Flag each one. If a point needs shouting to land, the naming or the structure should be carrying it instead.
1. Does the comment describe this code rather than other code? Flag narrative about unrelated implementations elsewhere ("not to be confused with X in path/y.go, which does the opposite"). A cross-reference is one line naming the file; more than that duplicates knowledge into two places that will disagree later.
1. Is the comment free of restating what the code already says? Flag any comment that adds nothing beyond the name, signature, or single line that follows it.
1. Does a comment justify keeping code that it also says cannot be reached? A comment explaining why dead-but-defensive code is retained is an argument for deleting the code. Flag the code, not just the comment.
1. Are comments free of these banned words, any capitalization or inflection? `genuine` · `real` · `win` · `grounded` · `honest` · `load-bearing` · `meaningfully` · `foot-gun`. They mark a comment insisting rather than informing; deleting the sentence is usually the fix, not a synonym. Exempt only when quoting a name the code doesn't control (`X-Real-IP`).

### General: Correctness

1. Is the code free of performance-sensitive issues? Does it avoid O(n) database transactions? Is the math efficient?
1. Is the code free of required manual intervention, such as a database update?
1. Do function callers correctly handle and report error conditions and/or sentinel return values?
1. Does each independently-failable operation have its own idempotency guard (hint, flag, watermark)? When new code appends a second operation after the first operation's guard has already committed, a failure in the second operation is silently swallowed — the guard prevents any retry. Each operation that must complete reliably needs its own guard, even when logically related.
1. Do tests check both the happy path and any failure paths? Do tests check cached data in addition to a fall-through fetch case?
1. Do tests avoid relying on special date values to protect against deleting test data? (Example: a test that sets dates to 2027 and cleans up all dates >= 2027 will fail once that year passes.)

### General: Design

1. If the code adds to a function longer than 300 lines, is there a todo.md item to address it?
1. Are the architectural tradeoffs written down? Prefer a design doc or todo.md with a one-line comment pointing at it; a short comment is fine when the tradeoff is local to one declaration. Flag tradeoffs that are recorded nowhere, and flag tradeoffs that grew into multi-paragraph comments instead of a doc (see General: Comments).
1. Was a test added as part of this change? If not and one should be added, recommend (in 2-3 sentences) what the test should do.
1. [cross-file] Does the code reuse existing functions rather than duplicating similar logic that exists elsewhere?
1. Does the code keep concerns within the correct file? E.g. model code should not appear in a repository file.
1. Is production code free of changes that exist solely to accommodate testing?
1. Does any comment admit the code breaks a rule, norm, or convention — and then the code does it anyway? Flag every one, however convincing the stated reason. Signals: "normally worth avoiding", "not ideal", "we shouldn't", "this is a hack", "technically wrong but", "the tradeoff is taken deliberately", "an accepted gap". The author's own justification is not review approval. Report the line, the rule broken, and the reason given; ask the user to accept it explicitly or add a todo.md item.
1. Does the code avoid splitting what should be a single atomic update across multiple HTTP endpoints? Separate requests can arrive out of order or partially fail, and living in two tables is not a reason to split. Flag such splits and recommend one endpoint that consolidates the writes in a single Postgres transaction.

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
1. Do not emit items that conclude "no issue", "this is fine", "safe", "acceptable". 
1. The report contains violations only, and this applies to the entire response — not just the numbered list. Never mention code you considered and decided not to flag: no "not flagged", "for the record", "worth noting", closing sections, asides, or explanations of why a rule did not fire. A dropped or disproven finding must not appear in the output in any form. If nothing violates any rule, the entire report is one line saying no violations were found.
