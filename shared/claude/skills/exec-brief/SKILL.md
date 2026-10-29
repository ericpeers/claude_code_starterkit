---
name: exec-brief
description: Write a one-page PDF brief that explains a technical finding to a reader outside the team. Use when asked for a brief, a one-pager, an executive summary, or a write-up to email or attach.
---

<!-- SPDX-License-Identifier: MIT -->

# Executive brief

A brief is one Letter page, rendered to PDF from HTML, for a reader who is technical but not on this team.
It answers one question or corrects one assumption with measured numbers.

## Before writing

1. Name the reader and the belief the brief answers. Example: "a backend engineer who assumes a cache always costs more memory."
2. List every term of art the reader may not know. Each one gets a one-sentence definition on first use.
   A reader fluent in other languages still needs the framework's words defined, such as "props" in React.
   Acronyms count, such as RSS. Define each one where it first appears, even if that is a table note.
3. Collect the numbers and how each was measured. A number with no method goes in the brief only with a caveat.

## Structure

Start from `template.html` in this directory. Keep this order:

1. **Title and subtitle.** The title names the subject. The subtitle gives the context: product, build, platform and month.
2. **Summary box.** Three or four short sentences with the answer and the headline numbers.
   Every number in it must match a table cell. A before/after pair comes from the same row.
   Give a spread as two ranges ("grew 160–754 MB before, 2–12 MB after"), not "from up to X to Y or less".
   Give each measure its own sentence. Quote time on the reader's device, not the test machine, when the table has that column.
3. **The assumption.** State the belief and explain the mechanism. Define terms here.
   Then say what the change was, in a short paragraph of plain mechanics. The reader needs it to see why the numbers moved.
4. **Result tables.** One table per question, each under its own heading. Group measures by the resource they describe.
   Every memory measure goes in one table, such as resident, peak and allocated. Time goes in another.
5. **Why section.** One section explains the results, as two to four bullets.
   Take its heading from the question or contrast the user raised, such as "Why the heap shrinks but resting RSS stays flat".
   Add a second why section only for a result the first does not explain.
6. **When it can go the other way.** The conditions under which the belief would hold. Start each bullet with its condition ("When a prop changes identity on every render, …"), so it does not read as a fact about this code.
7. **Method footer.** How the numbers were produced, in two or three sentences.

## Tables

- Compare a baseline and a change. Use the variant names as column labels, such as "Before" and "After" or "No cache" and "Cache".
- Use two header rows. The top row names the measure ("At rest", "Growth", "Time"), spanning its variants. The second row names the variants.
- Keep header labels to one or two words. Move the definition into the note under the table.
- When one value is shared by several rows, merge the cells with `rowspan`. Do not write "same as above".
- Mark the worse value `class="worse"` (red) and the better value `class="better"` (green). Leave equal values unmarked.
- When numbers are scaled to other hardware or conditions, give each target its own column group. Put the measured group first.
  Name the target device and its rounded multiplier in the group header ("Chromebook Plus (×2.5)"). Compute each cell from the exact ratio, such as 2,699 / 1,093, not the rounded multiplier.
- Name units after what the row measures. When rows mix keystrokes and clicks, say "per action".
- Show two or three significant figures: 764 ms, 4.3 ms, 0.60 MB. Do not show 763.8 ms, even when the arithmetic gives it.

## Notes under tables

- Notes are 5.6 pt, about four points below the section text. Keep each note to three lines. Five is the limit when the extra line earns it.
- Each note says how the numbers were measured and how many runs were averaged.
- Link to a source rather than explaining it in the text. Example: link a CPU name to its Geekbench search and a cache size to the vendor's spec page.
- Place one caveat where it applies. Do not repeat it under a second table.

## Method footer

- Describe what was compared in words the reader can follow ("the before build re-rendered every row; the after build skipped unchanged rows").
- Never cite commit hashes, branch names, ticket numbers or file paths. The reader cannot see them.

## Prose

Apply the "General: Comments" prose rules from `.claude/skills/review-code/SKILL.md`. The ones that matter most here:

- Each sentence has one actor. Rewrite "the props React stores" as "React stores the props".
- Keep sentences to 10–15 words. Split any sentence over 25 words.
- Avoid chains of parentheticals and em dashes. A parenthetical that carries content becomes its own sentence.
- State a conditional effect together with its condition.
- Use none of the banned words listed there.
- Say what hardware a scaled number models, rather than "4× slower than my machine". Name the target device and link the score the ratio comes from.
- Check every hardware or benchmark claim against a source before writing it. Hardware that was current two years ago is often wrong now.
- A short wry line suits a peer audience when the user asks for one. Do not invent one.

## Typography

The template sets these values. Keep them unless the user asks for a change.

| Element | Size | Notes |
|---|---|---|
| Body and summary | 10 pt | |
| Section heading | 11 pt, green | 16 pt above, 4 pt below |
| Section text | 8 pt | Indented 12 pt |
| Tables | 8 pt | |
| Notes under tables | 5.6 pt | Gray |
| Method footer | 6.6 pt | Gray, rule above |

Leave 14 pt under the subtitle and 18 pt under the summary box.

## Render and check

1. Write the HTML to the scratchpad, or wherever the user asks.
2. Render it with `node .claude/skills/exec-brief/render.mjs <abs.html> <abs.pdf>`. The script loads Playwright from the current directory's `node_modules`.
3. Run `pdfinfo <pdf> | grep Pages`. If the page count is over 1, cut words before shrinking fonts or margins.
4. Run `pdftoppm -png -r 80 <pdf> <prefix>` and look at the image. Check for wrapping in headers, empty space and leftover text from edits.
   Re-read each note against its table. The verb must match the arithmetic: a slower chip's times are multiplied by the ratio, not divided.
5. Report the PDF path, the headline numbers, and every claim you could not verify, such as a link that could not be fetched.
