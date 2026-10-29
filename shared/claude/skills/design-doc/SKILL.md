---
name: design-doc
description: Write or revise a design document in a repo's docs/. Use when asked to "write a design doc", "document the design", plan a system, or fix a doc that reads like notes.
---

<!-- SPDX-License-Identifier: MIT -->

## Before writing

1. Read the code the doc describes. Every path, function, flag and test name must exist; check each one.
2. Read existing docs in docs/ for overlap. Update or flag a stale doc; never cite it as if it were current.
3. Ask the user only for decisions the code can't answer. Record each decision once, where it applies.
4. Find where the repo already tracks this work, such as a checklist, findings doc or issue list. Name the work by those item numbers and link the tracker. Use "Stage N" labels only when no tracker exists.

## Structure

Sections in this order. Omit a section that has nothing to say.

1. **Problem:** Bullets of concrete gaps today. Each bullet names what fails and when.
2. **Solution:** One paragraph on the approach, then the stages, named by tracker item when one exists. Each stage gets one line on what it delivers.
3. **Terms:** Define each term the doc relies on: what it is, where it comes from, what it can do. Every later use follows its definition. Past about eight terms, group them under subheadings by area.
4. **How it works:** One run end to end, as numbered steps. Add a diagram when two or more parties exchange messages, or when it shows flow the steps can't.
5. **Components:** What gets installed or changed, and who owns or runs each piece.
6. **Security boundaries:** A table: layer, how it works, what it stops. State the trust model neutrally, without alarm.
7. **Details per area:** One table or subsection per repo, service or module. Each subsection opens with why the piece exists, then how it works.
8. **Setup:** The exact command, then numbered steps of what it does. Cover re-runs, rotation and recovery.
9. **Checking it:** Commands with a comment giving the expected output. Say what a bad result means.
10. **Later stages:** For each one: what it does, what existing code does today, and how that code changes for reuse. Title the section and each entry by tracker item when one exists, such as "Remaining checklist items" and "Item 6c".
11. **Future options:** Deferred choices, one paragraph each, with the cost or trigger to revisit.

Do not add a non-goals section or list things the design doesn't do. Scope belongs in one sentence in Solution.

## Writing

1. Every sentence has a subject and a verb. No bold fragment leads like "Staging only."
2. A bold lead that names a term or item ends with a colon: `**KEK:** The key encryption key.` A bold lead that is a full sentence ends with a period.
3. Keep sentences to 10-15 words; split any over 25. One parenthetical per sentence at most.
4. Explain each causal link. "X, so Y" must say why X leads to Y if a reader could miss it.
5. Write in present tense for behavior. Use future tense only for work in a later stage.
6. Be specific: numbers, paths, exact commands. "Valid for one hour", not "short-lived".
7. Mention a fact in the section where the reader needs it. A setup constraint goes in Setup, not the intro.
8. Describe only what the design uses. Don't name services, hostnames or tools that it doesn't touch.
9. State decisions without arguing for them. No rebuttals to objections nobody raised.
10. Tie each stage reference to something that changes in that stage. Don't label stable facts with a stage.
11. Avoid the banned words in review-code's General: Comments rules. No ALL-CAPS emphasis.

## Naming

1. Name users, services and files so a sysadmin knows their purpose: `githubci`, not `gha`.
2. Doc filenames in docs/ are lowercase. Match the separator the directory already uses.
3. Design out an option with one valid value. A script that only targets staging takes no role argument.

## Reusing existing code

1. Say what the existing code does today, split by where it runs.
2. Say which parts move, which stay, and how both callers use the shared piece. One copy of the logic, not two.
3. List changes needed in other repos as their own bullets, with the file and the fix.

## Before handing over

1. Run review-code's General: Comments rules over the doc. Fix every hit before showing it.
2. Re-read the doc top to bottom as a new reader. Every term, file and user should be introduced before use.
3. Report which design choices are new and not yet approved. Ask about those only.
