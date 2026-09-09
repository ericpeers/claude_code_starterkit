// SPDX-License-Identifier: MIT

package tests

// Three gates for the Comment style rules in CONVENTIONS.md. The baselines and
// the two environment variables are documented in CLAUDE_go.md under Testing.
// getRepoRoot resolves to the enclosing Go module, so the gates cover that
// module. They cover the whole repository only when go.mod sits at its top.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// styleWord is one entry in a guard's word list. name is the spelling reported
// in a failure. pattern also covers the inflections and any capitalization.
type styleWord struct {
	name    string
	pattern *regexp.Regexp
}

// bannedCommentWords may not appear in a comment or in documentation prose. The
// reason each one is out sits on its line. CONVENTIONS.md publishes the list.
var bannedCommentWords = []styleWord{
	{"by design", regexp.MustCompile(`(?i)\bby design\b`)},           // claims intent instead of describing behavior
	{"foot-gun", regexp.MustCompile(`(?i)\bfoot[- ]?guns?\b`)},       // metaphor; name the mistake a caller can make
	{"genuine", regexp.MustCompile(`(?i)\bgenuine(ly)?\b`)},          // filler; a no-op is a no-op
	{"grounded", regexp.MustCompile(`(?i)\bground(ed|ing)\b`)},       // metaphor with no fixed meaning in code
	{"honest", regexp.MustCompile(`(?i)\bhonest(ly|y)?\b`)},          // ascribes a motive to code
	{"load-bearing", regexp.MustCompile(`(?i)\bload[- ]?bearing\b`)}, // metaphor; say which caller depends on it
	{"meaningfully", regexp.MustCompile(`(?i)\bmeaningful(ly)?\b`)},  // vague intensifier; give the threshold
	{"oracle", regexp.MustCompile(`(?i)\boracles?\b`)},               // jargon, and confusable with the database
	{"pins", regexp.MustCompile(`(?i)\bpin(s|ned|ning)\b`)},          // jargon; a test asserts, checks, or requires
	{"real", regexp.MustCompile(`(?i)\breal\b`)},                     // filler in almost every use; delete it
	{"venue", regexp.MustCompile(`(?i)\bvenues?\b`)},                 // jargon; name the place or the service
	{"win", regexp.MustCompile(`(?i)\bwin(s|ning)?\b`)},              // metaphor; say which value takes precedence
}

// absoluteClaimWords overstate what the code enforces. The fix is to say what
// the code does.
var absoluteClaimWords = []styleWord{
	{"never", regexp.MustCompile(`(?i)\bnever\b`)},
	{"cannot", regexp.MustCompile(`(?i)\b(cannot|can not|can't)\b`)},
	{"impossible", regexp.MustCompile(`(?i)\bimpossible\b`)},
	{"guarantees", regexp.MustCompile(`(?i)\bguarantee(s|d|ing)?\b`)},
}

// styleExemptPhrases carry a banned word innocently: a proper name or a term of
// art. Add your project's domain vocabulary here, with a reason per line.
var styleExemptPhrases = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bx-real-ip\b`),             // the header name
	regexp.MustCompile(`(?i)\breal[- ]time\b`),          // the settled name for the latency class
	regexp.MustCompile(`(?i)\blast[- ]write[- ]wins\b`), // the settled name for the conflict rule
}

// maxSentenceWords is the ceiling from the Comment style section of CONVENTIONS.md.
// The target there is 10 to 15 words. This guard flags what runs well past it.
const maxSentenceWords = 25

// styleDetailCap limits how many hits one file prints in a failure message.
const styleDetailCap = 3

// styleReportEnv lifts that cap and prints baselined hits too.
const styleReportEnv = "COMMENT_STYLE_REPORT"

// styleBaselineEnv set to "write" rewrites each guard's baseline file from what
// the tree holds now.
const styleBaselineEnv = "COMMENT_STYLE_BASELINE"

// styleBaselineDir holds the baseline files, relative to the repository root.
const styleBaselineDir = "tests/testdata/comment_style"

// styleHit is one violation, located for the failure message.
type styleHit struct {
	line   int
	reason string
	text   string
}

// styleGuard names one rule and its baseline file. skip drops files the rule
// does not apply to, and is nil when it applies to every file in scope.
type styleGuard struct {
	name     string
	baseline string
	fix      string
	skip     func(rel string) bool
}

// instructionDoc reports whether a file directs the reader rather than
// describing code. "Do not assume a table is empty" is an order, not a claim.
func instructionDoc(rel string) bool {
	base := filepath.Base(rel)
	if strings.HasPrefix(base, "CLAUDE") || base == "CONVENTIONS.md" || base == "AGENTS.md" {
		return true
	}
	return strings.HasPrefix(rel, ".claude/")
}

var (
	bannedWordGuard = styleGuard{
		name:     "banned words",
		baseline: "banned_words.txt",
		fix: "rewrite the text, and do not swap in a synonym (the reason each word is out sits " +
			"on its line in bannedCommentWords)",
	}
	absoluteClaimGuard = styleGuard{
		name:     "absolute claims",
		baseline: "absolute_claims.txt",
		fix:      "say what the code does instead (see No absolute claims in CONVENTIONS.md)",
		skip:     instructionDoc,
	}
	sentenceLengthGuard = styleGuard{
		name:     "long sentences",
		baseline: "long_sentences.txt",
		fix:      "break the clause chain into separate sentences (see Comment style in CONVENTIONS.md)",
	}
)

// proseMarkers strip a line's comment marker, by file extension. Each language
// strips only its own, so --dry-run in a Python docstring keeps its dashes.
var proseMarkers = map[string]*regexp.Regexp{
	".go":  regexp.MustCompile(`^\s*(//+|\*+)`),
	".sql": regexp.MustCompile(`^\s*(--+|\*+)`),
	".py":  regexp.MustCompile(`^\s*#+`),
}

// markdownNonProse matches heading, quote, and table lines, which are labels
// rather than sentences.
var markdownNonProse = regexp.MustCompile(`^\s*([#>]|\|)`)

// listItem matches a bullet or a numbered item, in prose or in a comment. Each
// one opens its own block, so a list does not read as one long sentence.
var listItem = regexp.MustCompile(`^\s*([-*+]\s|\d+[.)]\s)`)

// sentenceBreak matches a full stop followed by whitespace. An abbreviation
// splits a sentence early, which undercounts words rather than overcounting.
var sentenceBreak = regexp.MustCompile(`[.!?]["')\]]*\s+`)

// codeLineStarts open a line that is comment text from its first character.
var codeLineStarts = []string{"//", "--", "#", "*", `"`, "'"}

// proseBlock is contiguous prose lines joined into one string, located at the
// first of them.
type proseBlock struct {
	line int
	text string
}

// stripMarker removes the comment marker from one line. It returns the leading
// whitespace width, counting a tab as four columns, and the trimmed text.
func stripMarker(ext, line string) (indent int, text string) {
	body := line
	if marker, ok := proseMarkers[ext]; ok {
		body = marker.ReplaceAllString(body, "")
	}
	for _, char := range body {
		switch char {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent, strings.TrimSpace(body)
		}
	}
	return 0, ""
}

// godocCodeLine reports whether godoc renders this comment line as a code block,
// which is a tab-indented body. Those carry tables and commands, not prose.
func godocCodeLine(ext, line string) bool {
	marker, ok := proseMarkers[ext]
	if !ok {
		return false
	}
	return strings.HasPrefix(marker.ReplaceAllString(line, ""), "\t")
}

// trailsCode reports whether the comment on this 1-indexed line follows code on
// the same line. Such a comment documents its own line, so it stands alone.
func trailsCode(src []string, line int) bool {
	if line < 1 || line > len(src) {
		return false
	}
	text := strings.TrimSpace(src[line-1])
	if text == "" {
		return false
	}
	for _, start := range codeLineStarts {
		if strings.HasPrefix(text, start) {
			return false
		}
	}
	return true
}

// blockBreak reports whether a line closes the block in progress, and whether it
// is dropped rather than joined. run is the current run's indent, or -1.
func blockBreak(file proseFile, ext string, cur proseLine, prev, run int) (breaks, drop bool) {
	if ext == ".md" {
		if markdownNonProse.MatchString(cur.text) {
			return true, true
		}
		_, text := stripMarker(ext, cur.text)
		return cur.num != prev+1 || listItem.MatchString(text), false
	}
	if godocCodeLine(ext, cur.text) {
		return true, true
	}
	indent, text := stripMarker(ext, cur.text)
	if text != "" && run >= 0 && indent > run {
		return true, true
	}
	return cur.num != prev+1 || listItem.MatchString(text) || trailsCode(file.lines, cur.num), false
}

// proseBlocks joins contiguous prose lines into blocks, dropping each marker. A
// line gap, list item, heading, indent step, or trailing code closes one.
func proseBlocks(file proseFile) []proseBlock {
	ext := filepath.Ext(file.rel)

	var blocks []proseBlock
	var current []string
	start, prev, run := 0, -2, -1

	// flush closes the block in progress. A closure rather than a method: it
	// moves three locals that would each have to be threaded in and back out.
	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, proseBlock{start, strings.Join(current, " ")})
			current = nil
		}
	}

	for _, cur := range file.prose {
		if cur.num != prev+1 {
			run = -1
		}
		breaks, drop := blockBreak(file, ext, cur, prev, run)
		if breaks {
			flush()
		}
		if drop {
			prev, run = cur.num, -1
			continue
		}
		indent, text := stripMarker(ext, cur.text)
		prev = cur.num
		if text == "" {
			flush()
			continue
		}
		if run < 0 {
			run = indent
		}
		text = listItem.ReplaceAllString(text, "")
		if len(current) == 0 {
			start = cur.num
		}
		current = append(current, text)
		if ext != ".md" && trailsCode(file.lines, cur.num) {
			flush()
		}
	}
	flush()
	return blocks
}

// wordHits reports every word in the list that one prose line carries.
func wordHits(cur proseLine, words []styleWord) []styleHit {
	scrubbed := cur.text
	for _, phrase := range styleExemptPhrases {
		scrubbed = phrase.ReplaceAllString(scrubbed, "")
	}
	var hits []styleHit
	for _, word := range words {
		if word.pattern.MatchString(scrubbed) {
			hits = append(hits, styleHit{cur.num, word.name, strings.TrimSpace(cur.text)})
		}
	}
	return hits
}

// collectWordHits scans every file in scope and groups the hits by file.
func collectWordHits(files []proseFile, words []styleWord) map[string][]styleHit {
	perFile := map[string][]styleHit{}
	for _, file := range files {
		for _, cur := range file.prose {
			if hits := wordHits(cur, words); len(hits) > 0 {
				perFile[file.rel] = append(perFile[file.rel], hits...)
			}
		}
	}
	return perFile
}

// collectLongSentences groups every over-long sentence by file.
func collectLongSentences(files []proseFile) map[string][]styleHit {
	perFile := map[string][]styleHit{}
	for _, file := range files {
		for _, block := range proseBlocks(file) {
			for _, sentence := range sentenceBreak.Split(block.text, -1) {
				words := len(strings.Fields(sentence))
				if words <= maxSentenceWords {
					continue
				}
				perFile[file.rel] = append(perFile[file.rel], styleHit{
					line:   block.line,
					reason: fmt.Sprintf("%d words", words),
					text:   truncate(sentence, 100),
				})
			}
		}
	}
	return perFile
}

// truncate shortens text for a failure message. It counts runes, so an em dash
// or an accent survives the cut.
func truncate(text string, width int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= width {
		return string(runes)
	}
	return string(runes[:width]) + "..."
}

// baselinePath returns the absolute path of one guard's baseline file.
func baselinePath(root string, guard styleGuard) string {
	return filepath.Join(root, filepath.FromSlash(styleBaselineDir), guard.baseline)
}

// readBaseline loads one guard's recorded counts, keyed by repo-relative path. A
// missing file reads as empty, so a fresh checkout enforces rather than passes.
func readBaseline(t *testing.T, root string, guard styleGuard) map[string]int {
	t.Helper()

	counts := map[string]int{}
	data, err := os.ReadFile(baselinePath(root, guard))
	if os.IsNotExist(err) {
		return counts
	}
	if err != nil {
		t.Fatalf("reading the %s baseline: %v", guard.name, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count, rel, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("%s baseline line %d is not \"count<TAB>path\": %q", guard.name, i+1, line)
		}
		parsed, convErr := strconv.Atoi(strings.TrimSpace(count))
		if convErr != nil {
			t.Fatalf("%s baseline line %d has a bad count: %v", guard.name, i+1, convErr)
		}
		counts[strings.TrimSpace(rel)] = parsed
	}
	return counts
}

// writeBaseline records what the tree holds now, sorted by path.
func writeBaseline(t *testing.T, root string, guard styleGuard, perFile map[string][]styleHit) {
	t.Helper()

	rels := sortedKeys(perFile)
	lines := []string{
		"# " + guard.name + " already in the tree, as count<TAB>path.",
		"# Rewrite the prose, then lower these with COMMENT_STYLE_BASELINE=write.",
	}
	for _, rel := range rels {
		lines = append(lines, fmt.Sprintf("%d\t%s", len(perFile[rel]), rel))
	}

	path := baselinePath(root, guard)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating the baseline directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("writing the %s baseline: %v", guard.name, err)
	}
	t.Logf("wrote %s with %d file(s)", relFromRoot(root, path), len(rels))
}

// scannedSet indexes the walked files by repo-relative path.
func scannedSet(files []proseFile) map[string]bool {
	scanned := make(map[string]bool, len(files))
	for _, file := range files {
		scanned[file.rel] = true
	}
	return scanned
}

// sortedKeys returns a map's keys in order, for a stable report.
func sortedKeys(perFile map[string][]styleHit) []string {
	rels := make([]string, 0, len(perFile))
	for rel := range perFile {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return rels
}

// styleViolations renders each file whose count differs from the baseline.
// scanned lets an entry for a deleted file read as a rename, not a cleanup.
func styleViolations(perFile map[string][]styleHit, baseline map[string]int, scanned map[string]bool) []string {
	detailCap := styleDetailCap
	if os.Getenv(styleReportEnv) != "" {
		detailCap = 0
	}

	var out []string
	for _, rel := range sortedKeys(perFile) {
		hits := perFile[rel]
		if len(hits) <= baseline[rel] {
			continue
		}
		sortHits(hits)
		shown, extra := hits, ""
		if detailCap > 0 && len(hits) > detailCap {
			shown = hits[:detailCap]
			extra = fmt.Sprintf("\n      ... %d more in this file (%s=1 with -v lists them all)",
				len(hits)-detailCap, styleReportEnv)
		}
		out = append(out, fmt.Sprintf("  %s: %d hit(s), baseline %d\n%s%s",
			rel, len(hits), baseline[rel], formatHits(rel, shown), extra))
	}

	var stale []string
	for rel, recorded := range baseline {
		if !scanned[rel] {
			continue // the file was deleted or renamed; its entry is dropped on the next write
		}
		if got := len(perFile[rel]); got < recorded {
			stale = append(stale, fmt.Sprintf("  %s: %d hit(s) but the baseline says %d", rel, got, recorded))
		}
	}
	sort.Strings(stale)
	return append(out, stale...)
}

// sortHits orders one file's hits for a stable failure message.
func sortHits(hits []styleHit) {
	// the comparator reads hits, which sort.Slice does not pass to a named function
	sort.Slice(hits, func(i, j int) bool { return hits[i].line < hits[j].line })
}

// formatHits renders one file's hits, one per line.
func formatHits(rel string, hits []styleHit) string {
	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, fmt.Sprintf("      %s:%d: %s in %q",
			rel, hit.line, hit.reason, truncate(hit.text, 120)))
	}
	return strings.Join(lines, "\n")
}

// runStyleGuard compares one guard's hits against its baseline, or rewrites that
// baseline when COMMENT_STYLE_BASELINE is set to write.
func runStyleGuard(t *testing.T, root string, guard styleGuard, files []proseFile, perFile map[string][]styleHit) {
	t.Helper()

	if guard.skip != nil {
		for rel := range perFile {
			if guard.skip(rel) {
				delete(perFile, rel)
			}
		}
	}

	if os.Getenv(styleReportEnv) != "" {
		reportAllHits(t, guard, perFile, readBaseline(t, root, guard))
	}
	if os.Getenv(styleBaselineEnv) == "write" {
		writeBaseline(t, root, guard, perFile)
		return
	}

	violations := styleViolations(perFile, readBaseline(t, root, guard), scannedSet(files))
	if len(violations) == 0 {
		return
	}
	t.Errorf("%s: %d file(s) differ from the baseline — %s, then record the lower counts with "+
		"%s=write:\n%s", guard.name, len(violations), guard.fix, styleBaselineEnv,
		strings.Join(violations, "\n"))
}

// reportAllHits logs every hit, baselined or not. Go prints test logs only under
// -v, so the report needs that flag to be seen.
func reportAllHits(t *testing.T, guard styleGuard, perFile map[string][]styleHit, baseline map[string]int) {
	t.Helper()

	total := 0
	var report []string
	for _, rel := range sortedKeys(perFile) {
		hits := perFile[rel]
		sortHits(hits)
		total += len(hits)
		report = append(report, fmt.Sprintf("%s: %d hit(s), baseline %d", rel, len(hits), baseline[rel]))
		report = append(report, formatHits(rel, hits))
	}
	t.Logf("%s: every hit found, baselined or not — %d in %d file(s):\n%s",
		guard.name, total, len(perFile), strings.Join(report, "\n"))
}

// TestNoBannedCommentWords enforces the vocabulary ban list over Go, SQL, and
// Python comments and over Markdown prose.
func TestNoBannedCommentWords(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, bannedWordGuard, files, collectWordHits(files, bannedCommentWords))
}

// TestNoAbsoluteClaims enforces the No absolute claims rule over the same files.
// Instruction documents are left out, since the words read as orders there.
func TestNoAbsoluteClaims(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, absoluteClaimGuard, files, collectWordHits(files, absoluteClaimWords))
}

// TestCommentSentenceLength flags sentences over maxSentenceWords. A long
// sentence is a clause chain to split.
func TestCommentSentenceLength(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, sentenceLengthGuard, files, collectLongSentences(files))
}
