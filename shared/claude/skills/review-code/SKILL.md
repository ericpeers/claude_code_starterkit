---
name: review-code
description : Performs a code review of the current changes. Use when code is complete and ready to commit, or when a user asks "review this code"
---

<!-- SPDX-License-Identifier: MIT -->

## Identifying changes to review
When reviewing code, start with the changes to be reviewed. If the user specifies a changeset or a specific file, constrain the review to that file or changeset. Otherwise, look for files that are locally modified or about to be committed:
1. Use git to check for staged changes: add them to the list of files to review. Do review source code (.go, .ts, .py, .sql). For markdown and documentation, apply only the General: Comments rules — skip Clarity, Correctness, and Design.
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
1. Do functions have comments? For 50+ line functions, do they describe the "what" and the "why" of the function in no more than 4 lines? For short functions, do they describe the what in 1-2 lines?
1. Do major sections of code have comments? If the code is complicated, does it have 1-2 line comments every 10-20 lines, walking the reader through the flow?
1. Are magic numbers that represent business rules or domain thresholds (regulatory limits, rate caps, timeouts, age/size boundaries) named as constants? Implementation-detail literals (byte offsets, fixed field widths, array indices) are acceptable with an inline comment instead.
1. [cross-file] Does the code follow the same naming patterns used elsewhere? E.g. is it a "userId" and not an "id".
1. Do variable names express a complete thought? Flag names that read as partial phrases
1. Does the code prefer a named function or method over a closure (anonymous function / lambda)? Flag any closure without a comment justifying why a named or helper function won't do — e.g. it must capture local state that can't be cleanly passed as a parameter.
1. Is the code free of continuation-based programming? Flag any pattern that passes a function to be partially evaluated and then returns data to a new function (continuation-passing style). Prefer straight-line code that computes a value and returns it directly over threading control through passed-in continuations.
1. Are variable names at least 3 characters? Flag one- and two-character names. Exempt: loop indices `i`, `j`, `k`; `_`; and the language's established short idioms.

### General: Comments

1. Is each comment proportional to the code it documents? Flag as too long: more than 2 comment lines on a single constant, variable, or struct field; more than 4 on a function; more than 10 on a type; more than 20 at file or package level. Quote the count and the declaration, and say which sentences to cut.
1. Are there too many comments? Flag any reviewed source file where total comment lines exceed 75% of code lines. Quote the count and the declaration, and say which sentences to cut.
1. If a single declaration needs a long explanation, that is a design smell rather than good documentation — the declaration is usually carrying too much. Flag it and recommend one of two fixes: simplify the design, or move the reasoning to a design doc and leave a one-line comment pointing at it.
1. Does the comment state the decision instead of arguing for it? Flag persuasive or defensive prose: pre-empting objections nobody raised, rebutting absent alternatives, or restating a point for emphasis. One or two sentences of "why" is the budget.
1. Are comments free of ALL-CAPS words used for emphasis? Flag each one.
1. Does the comment describe other code or how callers invoke it? Flag narrative about unrelated implementations elsewhere ("not to be confused with X in path/y.go, which does the opposite").
1. Is the comment free of restating what the code already says? Flag any comment that adds nothing beyond the name, signature, or single line that follows it.
1. Is the comment really necessary? Flag comments that describe a variable that is obvious. Consider renaming the variable if it is not obvious.
1. Could a variable comment be moved to the end of the line? Flag comments that take extra lines unless they are absolutely necessary.
1. Does the surrounding code have comments? Flag a comment that is out of place amongst its peers.
1. Does a comment justify keeping code that cannot be reached? Flag the code, not just the comment.
1. Are comments free of these banned words and phrases, any capitalization or inflection? `by design`, `foot-gun`, `genuine`, `grounded`, `honest`, `load-bearing`, `meaningfully`, `oracle`, `pins`, `real`, `venue`, `win`. Exempt only when quoting a proper name the code doesn't control (`X-Real-IP`, the Oracle database).
1. Does the comment lead with the normal case, with its exception right after? Flag a third fact placed between a rule and its exception.
1. Is the comment written for the reader of that surface? An API description (swagger, public docs) says what the caller sees and does. Flag server internals in it, such as check order, encryption, or audit logging.
1. Is a conditional behavior stated with its condition? Flag a sentence that states a conditional effect as if it always happens.
1. Are the words free of defensive qualifiers? Flag words that only rule out an unlikely reading ("own", "current", "calling").
1. Does the comment use a relative clause where a single adjective would suffice? Flag a clause that a single word can replace ("a run that is not a dry run" → "a normal run").
1. Does each sentence over 25 words have one actor? Flag a clause where a second noun acts ("the client that opts.Mode selects", "the rows Validate drops"). Quote the sentence and give a rewrite: "New builds a mailer based on cfg.Transport."
1. In a sentence over 25 words, does each pronoun have one possible referent? Flag "it", "this" or "which" when two or more earlier nouns could fit.
1. Does a test's doc lead with the behavior it protects? Flag a doc that describes the setup first and states the purpose later.
1. Does the prose use the API's names? Flag a synonym for a field, route, or error code ("expired holds" for `holds_released`) and suggest using the API name instead.
1. [cross-file] Is each caveat explained once? Flag the same edge case explained on several surfaces (swagger, function docs, design docs). Keep it where the code enforces it and point there, or drop it and rely on the test.
1. Are the sentences short enough to read? Target 10-15 words. Flag any sentence over 25 words. A sentence of 25 words or fewer passes this rule, whatever its commas, em dashes, or parentheticals. The fix is to break the clause chain into separate sentences. A parenthetical that carries content becomes its own sentence. Quote the offending sentence and give the split version. This rule covers markdown prose as much as code comments. Fenced code blocks and ASCII diagrams are exempt.

### General: Correctness

1. Is the code free of performance-sensitive issues? Does it avoid O(n) database transactions? Is the math efficient?
1. Is the code free of required manual intervention, such as a database update?
1. Do function callers correctly handle and report error conditions and/or sentinel return values?
1. Does each independently-failable operation have its own idempotency guard (hint, flag, watermark)? Each operation that must complete reliably needs its own guard, even when logically related.
1. Do tests check both the happy path and any failure paths? Do tests check cached data in addition to a fall-through fetch case?
1. Do tests avoid relying on special date values to protect against deleting test data? (Example: a test that sets dates to 2027 and cleans up all dates >= 2027 will fail once that year passes.)

### General: Design

1. If the code adds to a function longer than 300 lines, has the human manually added a todo.md item to address it?
1. Are the architectural tradeoffs written down? Prefer a design doc with a one-line comment pointing at it; a short comment is fine when the tradeoff is local to one declaration. Flag tradeoffs that are recorded nowhere, and flag tradeoffs that grew into multi-paragraph comments instead of a doc (see General: Comments).
1. Was a test added as part of this change? If not and one should be added, recommend (in 2-3 sentences) what the test should do.
1. [cross-file] Does the code reuse existing functions rather than duplicating similar logic that exists elsewhere?
1. Does the code keep concerns within the correct file? E.g. model code should not appear in a repository file.
1. Is production code free of changes that exist solely to accommodate testing?
1. Does any comment admit the code breaks a rule, norm, or convention — and then the code does it anyway? Flag every one, however convincing the stated reason. Signals: "normally worth avoiding", "not ideal", "we shouldn't", "this is a hack", "technically wrong but", "the tradeoff is taken deliberately", "an accepted gap". The author's own justification is not review approval. Report the line, the rule broken, and the reason given; ask the human to accept it explicitly or to manually add a todo.md item.
1. Does the code avoid splitting what should be a single atomic update across multiple HTTP endpoints? Separate requests can arrive out of order or partially fail, and living in two tables is not a reason to split. Flag such splits and recommend one endpoint that consolidates the writes in a single Postgres transaction.

### General: Tests
1. Does a test check something that its own comments describe as wrong, temporary, or "current"? Does a test greenlight what should be a failure just because it is current behavior? Flag if so.
1. Does a test check a number the code reported, when the underlying fact could have been read directly instead?



### Language: JavaScript / React
Apply only when reviewing *.ts, *.tsx

1. If the code uses `useMemo`, does it include a comment explaining why inline or functional calculation isn't adequate and what the performance cost/sensitivity is? A single well-justified `useMemo` is acceptable; flag any without justification.
1. Is `useMemo` usage free of chains (two or more `useMemo` calls where one depends on another)? Chained `useMemo` is never acceptable regardless of justification — flag it and recommend restructuring.


### Language: Go
Apply only when reviewing *.go files

1. Do new API endpoints include appropriate swagger documentation describing what the endpoint does?
1. Is each swagger `@Description` 1–3 sentences? Flag field meanings that belong on the model fields, and error cases that belong on `@Failure` lines.

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
