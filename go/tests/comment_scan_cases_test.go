// SPDX-License-Identifier: MIT

package tests

// Fixtures for the scanners, the block splitter, and the baseline comparison. A
// scanner that returned nothing would make every guard pass on any input.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// proseScanCase is one fixture for the per-language scanners: source text, and
// the prose lines the scanner must return from it.
type proseScanCase struct {
	name string
	file string
	src  string
	want []proseLine
}

// proseScanCases give each scanner a banned word in a comment and the same word
// in code. A scanner reading too much or too little is caught either way.
var proseScanCases = []proseScanCase{
	{
		name: "go skips the license header, the directives, and the swag annotations",
		file: "sample.go",
		src: "// SPDX-License-Identifier: MIT\n" +
			"\n" +
			"// Package sample is real.\n" +
			"package sample\n" +
			"\n" +
			"//go:generate stringer -type=T\n" +
			"// @Summary a real endpoint\n" +
			"// a real comment\n" +
			"const Kind = \"real\"\n",
		want: []proseLine{{3, "// Package sample is real."}, {8, "// a real comment"}},
	},
	{
		name: "sql reads comments, not string literals",
		file: "sample.sql",
		src: "SELECT 'a real value' AS label; -- a real comment\n" +
			"/* a real block */\n" +
			"INSERT INTO t VALUES ('real');\n",
		want: []proseLine{{1, "-- a real comment"}, {2, " a real block "}},
	},
	{
		name: "python reads comments and docstrings, not string literals",
		file: "sample.py",
		src: "\"\"\"a real docstring\n" +
			"still the docstring\"\"\"\n" +
			"label = \"a real value  # not a comment\"\n" +
			"total = 1  # a real comment\n",
		want: []proseLine{
			{1, "\"\"\"a real docstring"},
			{2, "still the docstring\"\"\""},
			{4, "# a real comment"},
		},
	},
	{
		name: "python skips a triple-quoted literal in code and keeps the comment after it",
		file: "sample.py",
		src: "value = \"\"\"a real literal\"\"\"  # a real comment\n" +
			"def f():\n" +
			"    \"\"\"An indented docstring.\"\"\"\n",
		want: []proseLine{
			{1, "# a real comment"},
			{3, "    \"\"\"An indented docstring.\"\"\""},
		},
	},
	{
		name: "python skips a multi-line literal assigned in code",
		file: "sample.py",
		src: "query = \"\"\"\n" +
			"SELECT a real column\n" +
			"\"\"\"\n" +
			"total = 1  # a real comment\n",
		want: []proseLine{{4, "# a real comment"}},
	},
	{
		name: "markdown reads prose, not fenced blocks or inline code",
		file: "sample.md",
		src: "a real sentence\n" +
			"```go\n" +
			"const Real = \"real\"\n" +
			"```\n" +
			"the `real` identifier\n",
		want: []proseLine{{1, "a real sentence"}, {5, "the  identifier"}},
	},
}

// TestProseScannersReadOnlyComments drives each scanner against its fixture.
func TestProseScannersReadOnlyComments(t *testing.T) {
	t.Parallel()

	for _, tc := range proseScanCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scanned, err := scanSource(tc.file, tc.file, []byte(tc.src))
			if err != nil {
				t.Fatalf("scanning %s: %v", tc.file, err)
			}
			got := scanned.prose
			if len(got) != len(tc.want) {
				t.Fatalf("got %d line(s), want %d: %+v", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("line %d: got %+v, want %+v", i, got[i], want)
				}
			}
		})
	}
}

// TestScanSourceReportsUnparsedGo checks that broken Go source returns an error
// rather than stopping the walk. One such file must not disable every gate.
func TestScanSourceReportsUnparsedGo(t *testing.T) {
	t.Parallel()

	if _, err := scanSource("broken.go", "broken.go", []byte("package sample\nfunc (\n")); err == nil {
		t.Fatal("got no error for source that does not parse, want one")
	}
}

// proseBlockCase is one fixture for the block splitter.
type proseBlockCase struct {
	name string
	file string
	src  string
	want []proseBlock
}

// proseBlockCases cover the lines that are not prose: a usage block under a
// docstring, a godoc code block, and Markdown bullets.
var proseBlockCases = []proseBlockCase{
	{
		name: "python usage lines stay out of the docstring prose",
		file: "sample.py",
		src: "\"\"\"Seed loader.\n" +
			"\n" +
			"Usage:\n" +
			"    --dry-run   report without writing\n" +
			"\"\"\"\n",
		want: []proseBlock{
			{1, "\"\"\"Seed loader."},
			{3, "Usage:"},
			{5, "\"\"\""},
		},
	},
	{
		name: "go tab-indented lines stay out of the prose above them",
		file: "sample.go",
		src: "package sample\n" +
			"\n" +
			"// Run it:\n" +
			"//\n" +
			"//\tgo test ./tests/\n" +
			"func Sample() {}\n",
		want: []proseBlock{{3, "Run it:"}},
	},
	{
		name: "a go comment of tab-indented lines only holds no prose",
		file: "sample.go",
		src: "package sample\n" +
			"\n" +
			"//\tid   = the primary key\n" +
			"//\tname = the display name\n" +
			"func Sample() {}\n",
		want: nil,
	},
	{
		name: "a comment trailing code stands alone",
		file: "sample.go",
		src: "package sample\n" +
			"\n" +
			"// the upload ceiling\n" +
			"const MaxUpload = 4096 // bytes, matched to the proxy limit\n",
		want: []proseBlock{{3, "the upload ceiling"}, {4, "bytes, matched to the proxy limit"}},
	},
	{
		name: "markdown bullets each open a block and drop the marker",
		file: "sample.md",
		src: "a lead in\n" +
			"* first bullet\n" +
			"  wrapped onto a second line\n" +
			"- second bullet\n",
		want: []proseBlock{
			{1, "a lead in"},
			{2, "first bullet wrapped onto a second line"},
			{4, "second bullet"},
		},
	},
	{
		name: "markdown headings and table rows are dropped",
		file: "sample.md",
		src: "## A heading\n" +
			"| column | type |\n" +
			"|---|---|\n" +
			"prose after the table\n",
		want: []proseBlock{{4, "prose after the table"}},
	},
}

// TestProseBlocksSplitNonProse drives the splitter against each fixture.
func TestProseBlocksSplitNonProse(t *testing.T) {
	t.Parallel()

	for _, tc := range proseBlockCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scanned, err := scanSource(tc.file, tc.file, []byte(tc.src))
			if err != nil {
				t.Fatalf("scanning %s: %v", tc.file, err)
			}
			prose := scanned.prose
			file := proseFile{
				rel:   tc.file,
				path:  tc.file,
				lines: strings.Split(tc.src, "\n"),
				prose: prose,
			}
			got := proseBlocks(file)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d block(s), want %d: %+v", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("block %d: got %+v, want %+v", i, got[i], want)
				}
			}
		})
	}
}

// TestStyleGuardsFindSeededHits checks that each word list still reports a hit
// on text that breaks its rule. A regexp typo would otherwise pass in silence.
func TestStyleGuardsFindSeededHits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		words []styleWord
		text  string
		want  string
	}{
		{"banned word", bannedCommentWords, "// this is the real record id", "real"},
		{"banned inflection", bannedCommentWords, "// the newest signature wins", "win"},
		{"absolute claim", absoluteClaimWords, "// the id can never be zero", "never"},
		{"absolute claim inflection", absoluteClaimWords, "// this guarantees ordering", "guarantees"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hits := wordHits(proseLine{1, tc.text}, tc.words)
			if len(hits) != 1 || hits[0].reason != tc.want {
				t.Fatalf("got %+v, want one hit on %q", hits, tc.want)
			}
		})
	}
}

// TestStyleExemptPhrasesSurvive checks the phrases that carry a banned word
// without the use the rule is after.
func TestStyleExemptPhrasesSurvive(t *testing.T) {
	t.Parallel()

	exempt := []string{
		"// the X-Real-IP header names the client",
		"// updates apply last-write-wins",
		"// the response is served in real time",
	}
	for _, text := range exempt {
		if hits := wordHits(proseLine{1, text}, bannedCommentWords); len(hits) > 0 {
			t.Errorf("%q reported %+v, want no hit", text, hits)
		}
	}
}

// nStyleHits builds n placeholder hits, so a comparison case can state a count
// on its own.
func nStyleHits(n int) []styleHit {
	out := make([]styleHit, 0, n)
	for i := range n {
		out = append(out, styleHit{line: i + 1, reason: "banned", text: "a line"})
	}
	return out
}

// TestStyleViolationsComparesBothDirections drives the baseline comparison,
// which is what decides whether a gate passes. A bug here shows up in no
// scanner fixture, and would report every file as clean.
func TestStyleViolationsComparesBothDirections(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		perFile  map[string][]styleHit
		baseline map[string]int
		scanned  map[string]bool
		want     int
		contains string
	}{
		{
			name:     "above the baseline reports",
			perFile:  map[string][]styleHit{"a.go": nStyleHits(3)},
			baseline: map[string]int{"a.go": 1},
			scanned:  map[string]bool{"a.go": true},
			want:     1,
			contains: "3 hit(s), baseline 1",
		},
		{
			name:     "at the baseline is silent",
			perFile:  map[string][]styleHit{"a.go": nStyleHits(2)},
			baseline: map[string]int{"a.go": 2},
			scanned:  map[string]bool{"a.go": true},
			want:     0,
		},
		{
			name:     "below the baseline reports until the count is recorded",
			perFile:  map[string][]styleHit{"a.go": nStyleHits(1)},
			baseline: map[string]int{"a.go": 4},
			scanned:  map[string]bool{"a.go": true},
			want:     1,
			contains: "1 hit(s) but the baseline says 4",
		},
		{
			name:     "a fully cleaned file reports until the entry is dropped",
			perFile:  map[string][]styleHit{},
			baseline: map[string]int{"a.go": 2},
			scanned:  map[string]bool{"a.go": true},
			want:     1,
			contains: "0 hit(s) but the baseline says 2",
		},
		{
			name:     "an entry for a file the walk never saw is ignored",
			perFile:  map[string][]styleHit{},
			baseline: map[string]int{"gone.go": 2},
			scanned:  map[string]bool{},
			want:     0,
		},
		{
			name:     "a new file with no entry reports",
			perFile:  map[string][]styleHit{"new.go": nStyleHits(1)},
			baseline: map[string]int{},
			scanned:  map[string]bool{"new.go": true},
			want:     1,
			contains: "1 hit(s), baseline 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := styleViolations(tc.perFile, tc.baseline, tc.scanned)
			if len(got) != tc.want {
				t.Fatalf("got %d entry(s), want %d: %q", len(got), tc.want, got)
			}
			if tc.contains != "" && !strings.Contains(strings.Join(got, "\n"), tc.contains) {
				t.Errorf("got %q, want text containing %q", got, tc.contains)
			}
		})
	}
}

// TestBaselineRoundTrip writes a baseline and reads it back. A format change on
// one side would otherwise leave the other side matching nothing.
func TestBaselineRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	guard := styleGuard{name: "round trip", baseline: "round_trip.txt"}
	writeBaseline(t, root, guard, map[string][]styleHit{
		"a.go":     nStyleHits(2),
		"dir/b.md": nStyleHits(1),
	})

	got := readBaseline(t, root, guard)
	want := map[string]int{"a.go": 2, "dir/b.md": 1}
	if len(got) != len(want) {
		t.Fatalf("read %d entry(s), want %d: %+v", len(got), len(want), got)
	}
	for rel, count := range want {
		if got[rel] != count {
			t.Errorf("%s: read %d, want %d", rel, got[rel], count)
		}
	}
}

// TestMissingBaselineEnforces checks that an absent file reads as no recorded
// hits. A fresh checkout must enforce rather than pass.
func TestMissingBaselineEnforces(t *testing.T) {
	t.Parallel()

	guard := styleGuard{name: "absent", baseline: "absent.txt"}
	if got := readBaseline(t, t.TempDir(), guard); len(got) != 0 {
		t.Errorf("got %+v, want no entries", got)
	}
}

// TestInstructionDocsAreSkipped checks which files the absolute-claim guard
// leaves alone. A wrong answer here drops a file from the gate in silence.
func TestInstructionDocsAreSkipped(t *testing.T) {
	t.Parallel()

	exempt := []string{"CLAUDE.md", "CLAUDE_go.md", "CONVENTIONS.md", "AGENTS.md", ".claude/commands/x.md"}
	for _, rel := range exempt {
		if !instructionDoc(rel) {
			t.Errorf("%s is enforced, want skipped", rel)
		}
	}

	enforced := []string{"README.md", "docs/design.md", "internal/app/main.go", "tests/setup_test.go"}
	for _, rel := range enforced {
		if instructionDoc(rel) {
			t.Errorf("%s is skipped, want enforced", rel)
		}
	}
}

// missedTrackedFiles returns the tracked files in scope that the walk did not
// scan. A file deleted or renamed in the working tree is not there to walk.
func missedTrackedFiles(root string, tracked []string, scanned map[string]bool) ([]string, error) {
	var missing []string
	for _, rel := range tracked {
		if !inScope(rel) || scanned[rel] {
			continue
		}
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		missing = append(missing, rel)
	}
	return missing, nil
}

// TestProseScanCoversTrackedFiles checks the walk reaches every tracked file in
// scope, under the Go module the gates run in. A walk that stopped early would
// quietly shrink the next baseline.
func TestProseScanCoversTrackedFiles(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	tracked := trackedFiles(t, root)
	missing, err := missedTrackedFiles(root, tracked, scannedSet(loadProseFiles(t, root)))
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("the walk missed %d tracked file(s) that inScope accepts:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestMissedTrackedFilesSkipsDeletedFiles fails when the coverage check reports
// a deleted file, passes an unscanned one, or hides a stat failure.
func TestMissedTrackedFilesSkipsDeletedFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{"scanned.go", "unscanned.go", "locked/hidden.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package p\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scanned := map[string]bool{"scanned.go": true}

	missing, err := missedTrackedFiles(root, []string{"scanned.go", "unscanned.go", "deleted.go"}, scanned)
	if err != nil || len(missing) != 1 || missing[0] != "unscanned.go" {
		t.Errorf("missedTrackedFiles = %q, %v; want [unscanned.go] and no error", missing, err)
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode, so the stat failure cannot be staged")
	}
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	// Cleanup takes a func value. TempDir's removal needs the directory readable again.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := missedTrackedFiles(root, []string{"locked/hidden.go"}, scanned); err == nil {
		t.Error("missedTrackedFiles hid a stat failure, want an error")
	}
}
