// SPDX-License-Identifier: MIT

package tests

// Guarded regions: code between guard markers changes only with the human's
// approval. See Files only the human edits in CONVENTIONS.md.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// guardManifestPath is the human-owned list of guarded regions and their approved hashes.
var guardManifestPath = filepath.Join("tests", "testdata", "guards.txt")

// guardHashReportEnv, when set, makes TestGuardedRegions log each region's hash.
const guardHashReportEnv = "GUARD_HASH_REPORT"

// Marker comments that open and close a guarded region. Each is followed by the region's name.
const (
	guardBeginMarker = "guard:begin"
	guardEndMarker   = "guard:end"
)

// guardRegion is one guarded span of Go code.
type guardRegion struct {
	name string
	path string // repo-relative, forward slashes
	line int    // line of the begin marker
	hash string
}

// TestGuardedRegions fails when code between guard markers differs from the approved hash in the
// manifest. Comment and whitespace edits keep the hash. Only the human edits the manifest.
func TestGuardedRegions(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)

	var regions []guardRegion
	var problems []string
	// Test files are scanned too, so a test table can be guarded.
	for _, file := range loadGoFiles(t, root) {
		found, markerProblems := parseGuardRegions(file)
		regions = append(regions, found...)
		problems = append(problems, markerProblems...)
	}

	if os.Getenv(guardHashReportEnv) != "" {
		for _, region := range regions {
			t.Logf("%s %s %s", region.name, region.path, region.hash)
		}
	}

	problems = append(problems, compareGuardManifest(regions, readGuardManifest(t, root))...)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Errorf("found %d guarded-region problem(s). Guarded code changes only with the human's "+
			"approval. Ask the human to review the change and update %s:\n%s",
			len(problems), guardManifestPath, strings.Join(problems, "\n"))
	}
}

// TestParseGuardRegions checks marker pairing and which edits change a region's hash.
func TestParseGuardRegions(t *testing.T) {
	t.Parallel()

	base := "package p\n\n// guard:begin r1\nvar x = 1\n// guard:end r1\n"
	baseRegions, baseProblems := parseGuardRegions(parseGuardFixture(t, base))
	if len(baseRegions) != 1 || len(baseProblems) != 0 {
		t.Fatalf("base: regions %v, problems %v; want one region and no problems", baseRegions, baseProblems)
	}

	hashCases := []struct {
		name     string
		src      string
		wantSame bool
	}{
		{"comment added", "package p\n\n// guard:begin r1\n// one\nvar x = 1 // two\n// guard:end r1\n", true},
		{"whitespace changed", "package p\n\n// guard:begin r1\n\n\tvar   x = 1\n\n// guard:end r1\n", true},
		{"value changed", "package p\n\n// guard:begin r1\nvar x = 2\n// guard:end r1\n", false},
		{"name changed", "package p\n\n// guard:begin r1\nvar y = 1\n// guard:end r1\n", false},
	}
	for _, tc := range hashCases {
		regions, problems := parseGuardRegions(parseGuardFixture(t, tc.src))
		if len(regions) != 1 || len(problems) != 0 {
			t.Errorf("%s: regions %v, problems %v; want one region and no problems", tc.name, regions, problems)
			continue
		}
		if same := regions[0].hash == baseRegions[0].hash; same != tc.wantSame {
			t.Errorf("%s: hash unchanged = %v, want %v", tc.name, same, tc.wantSame)
		}
	}

	markerCases := []struct {
		name string
		src  string
	}{
		{"begin without end", "package p\n// guard:begin r1\nvar x = 1\n"},
		{"end without begin", "package p\nvar x = 1\n// guard:end r1\n"},
		{"nested", "package p\n// guard:begin r1\n// guard:begin r2\nvar x = 1\n// guard:end r2\n// guard:end r1\n"},
		{"end name differs", "package p\n// guard:begin r1\nvar x = 1\n// guard:end r2\n"},
		{"missing name", "package p\n// guard:begin\nvar x = 1\n// guard:end\n"},
		{"duplicate name", "package p\n// guard:begin r1\nvar x = 1\n// guard:end r1\n// guard:begin r1\nvar y = 1\n// guard:end r1\n"},
	}
	for _, tc := range markerCases {
		if _, problems := parseGuardRegions(parseGuardFixture(t, tc.src)); len(problems) == 0 {
			t.Errorf("%s: want a marker problem, got none", tc.name)
		}
	}
}

// parseGuardFixture parses one fixture source as p.go.
func parseGuardFixture(t *testing.T, src string) goFile {
	t.Helper()
	file, err := parseGoSource("p.go", "p.go", src)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// parseGuardRegions returns the guarded regions in one Go file, and any malformed markers.
func parseGuardRegions(file goFile) ([]guardRegion, []string) {
	relPath, fset := file.rel, file.fset
	src := []byte(strings.Join(file.lines, "\n"))

	var regions []guardRegion
	var problems []string
	seen := map[string]bool{}
	openName, openLine, openOffset := "", 0, 0
	for _, group := range file.syntax.Comments {
		for _, comment := range group.List {
			kind, name := guardMarker(comment.Text)
			if kind == "" {
				continue
			}
			pos := fset.Position(comment.Pos())
			where := fmt.Sprintf("  %s:%d:", relPath, pos.Line)
			switch {
			case name == "":
				problems = append(problems, where+" guard marker has no region name")
			case kind == guardBeginMarker && openName != "":
				problems = append(problems, fmt.Sprintf("%s region %q opens inside region %q", where, name, openName))
			case kind == guardBeginMarker && seen[name]:
				problems = append(problems, fmt.Sprintf("%s region name %q is used twice", where, name))
			case kind == guardBeginMarker:
				seen[name] = true
				openName, openLine, openOffset = name, pos.Line, fset.Position(comment.End()).Offset
			case openName == "":
				problems = append(problems, fmt.Sprintf("%s region %q ends without a begin", where, name))
			case name != openName:
				problems = append(problems, fmt.Sprintf("%s end names %q, but region %q is open", where, name, openName))
			default:
				regions = append(regions, guardRegion{
					name: name, path: relPath, line: openLine,
					hash: hashGoTokens(src[openOffset:pos.Offset]),
				})
				openName = ""
			}
		}
	}
	if openName != "" {
		problems = append(problems, fmt.Sprintf("  %s:%d: region %q has no end marker", relPath, openLine, openName))
	}
	return regions, problems
}

// guardMarker returns the marker kind and region name in one comment, or "" for other comments.
func guardMarker(commentText string) (kind, name string) {
	text := strings.TrimSpace(strings.TrimPrefix(commentText, "//"))
	for _, marker := range []string{guardBeginMarker, guardEndMarker} {
		if text == marker || strings.HasPrefix(text, marker+" ") {
			return marker, strings.TrimSpace(strings.TrimPrefix(text, marker))
		}
	}
	return "", ""
}

// hashGoTokens hashes the Go tokens in a fragment. Comments and whitespace do not count.
func hashGoTokens(fragment []byte) string {
	fset := token.NewFileSet()
	var scan scanner.Scanner
	scan.Init(fset.AddFile("", fset.Base(), len(fragment)), fragment, nil, 0)
	digest := sha256.New()
	for {
		_, tok, lit := scan.Scan()
		if tok == token.EOF {
			break
		}
		// An inserted semicolon carries "\n" as its literal. Only the token itself counts.
		text := tok.String()
		if tok.IsLiteral() {
			text = lit
		}
		fmt.Fprintf(digest, "%s\x00", text)
	}
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

// readGuardManifest returns the approved regions from the manifest, keyed by name.
func readGuardManifest(t *testing.T, root string) map[string]guardRegion {
	t.Helper()
	file, err := os.Open(filepath.Join(root, guardManifestPath))
	if err != nil {
		t.Fatalf("open the guard manifest: %v", err)
	}
	defer file.Close()

	approved := map[string]guardRegion{}
	lines := bufio.NewScanner(file)
	for lineNum := 1; lines.Scan(); lineNum++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("%s:%d: want \"name path hash\", got %q", guardManifestPath, lineNum, line)
		}
		approved[fields[0]] = guardRegion{name: fields[0], path: fields[1], hash: fields[2]}
	}
	if err := lines.Err(); err != nil {
		t.Fatalf("read the guard manifest: %v", err)
	}
	return approved
}

// compareGuardManifest lists each region that differs from, or is missing from, the manifest.
func compareGuardManifest(regions []guardRegion, approved map[string]guardRegion) []string {
	var problems []string
	inCode := map[string]bool{}
	for _, region := range regions {
		inCode[region.name] = true
		where := fmt.Sprintf("  %s:%d: region %q", region.path, region.line, region.name)
		want, ok := approved[region.name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s is not in the manifest (hash %s)", where, region.hash))
		case want.path != region.path:
			problems = append(problems, fmt.Sprintf("%s moved from %s", where, want.path))
		case want.hash != region.hash:
			problems = append(problems, fmt.Sprintf("%s changed (old hash %s, new hash %s)", where, want.hash, region.hash))
		}
	}
	for name, want := range approved {
		if !inCode[name] {
			problems = append(problems, fmt.Sprintf("  %s: region %q is in the manifest but not in the code", want.path, name))
		}
	}
	return problems
}

// TestCompareGuardManifest fails when a region that is new, moved, changed, or
// gone from the code passes the manifest check.
func TestCompareGuardManifest(t *testing.T) {
	t.Parallel()
	approved := map[string]guardRegion{
		"same":    {name: "same", path: "a.go", hash: "h1"},
		"moved":   {name: "moved", path: "a.go", hash: "h2"},
		"changed": {name: "changed", path: "a.go", hash: "h3"},
		"removed": {name: "removed", path: "a.go", hash: "h4"},
	}
	regions := []guardRegion{
		{name: "same", path: "a.go", line: 1, hash: "h1"},
		{name: "moved", path: "b.go", line: 2, hash: "h2"},
		{name: "changed", path: "a.go", line: 3, hash: "h9"},
		{name: "added", path: "a.go", line: 4, hash: "h5"},
	}
	got := compareGuardManifest(regions, approved)
	sort.Strings(got)
	want := []string{
		`  a.go: region "removed" is in the manifest but not in the code`,
		`  a.go:3: region "changed" changed (old hash h3, new hash h9)`,
		`  a.go:4: region "added" is not in the manifest (hash h5)`,
		`  b.go:2: region "moved" moved from a.go`,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("compareGuardManifest =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
