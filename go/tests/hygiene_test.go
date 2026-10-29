// SPDX-License-Identifier: MIT

package tests

// Source and repository hygiene gates: debug markers, line length, dead-variable
// discards, test placement, read-only git in tests, database dumps, and shell
// pipelines.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// hackFixmeMarker tags temporary debugging code that must not be committed. A
// bare FIXME is a backlog note and does not match.
const hackFixmeMarker = "HACK " + "FIXME"

// TestNoHackFixmeMarkers fails on the marker in production source, so the
// pre-commit hook blocks the commit that would carry it.
func TestNoHackFixmeMarkers(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, file := range productionFiles(loadGoFiles(t, getRepoRoot(t))) {
		violations = append(violations, hackFixmeLines(file)...)
	}
	if len(violations) > 0 {
		t.Errorf("found %d %q marker(s). Remove the temporary code before committing:\n%s",
			len(violations), hackFixmeMarker, strings.Join(violations, "\n"))
	}
}

// hackFixmeLines reports each line of one file that carries the marker.
func hackFixmeLines(file goFile) []string {
	var violations []string
	for i, line := range file.lines {
		if strings.Contains(line, hackFixmeMarker) {
			violations = append(violations, fmt.Sprintf("  %s:%d: %s", file.rel, i+1, strings.TrimSpace(line)))
		}
	}
	return violations
}

// maxLineLength is the ceiling on a production Go line, in columns. A tab counts as tabWidth.
const maxLineLength = 250

// lineLengthOverrideMarker exempts the line directly below it. Reserve it for a
// line that resists wrapping, such as a long struct tag or URL.
const lineLengthOverrideMarker = "//no line_len check"

// enumCommentLines returns the lines of trailing comments on const declarations.
// swag copies them into swagger.json as x-enum-comments, so they stay on one line.
func enumCommentLines(file goFile) map[int]bool {
	exempt := map[int]bool{}
	for _, decl := range file.syntax.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			if valueSpec, ok := spec.(*ast.ValueSpec); ok && valueSpec.Comment != nil {
				exempt[file.fset.Position(valueSpec.Comment.Pos()).Line] = true
			}
		}
	}
	return exempt
}

// longLines reports each line of one file over maxLineLength. Enum comments,
// swag annotations, and a line under the override marker are exempt.
func longLines(file goFile) []string {
	exempt := enumCommentLines(file)
	var violations []string
	for i, line := range file.lines {
		width := displayWidth(line)
		switch {
		case width <= maxLineLength, exempt[i+1]:
		case strings.HasPrefix(strings.TrimSpace(line), "// @"):
		case i > 0 && strings.Contains(file.lines[i-1], lineLengthOverrideMarker):
		default:
			violations = append(violations, fmt.Sprintf("  %s:%d: %d columns (limit %d). Wrap it, or put %q on the line above",
				file.rel, i+1, width, maxLineLength, lineLengthOverrideMarker))
		}
	}
	return violations
}

// TestMaxLineLength fails on a production Go line longer than maxLineLength.
func TestMaxLineLength(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, file := range productionFiles(loadGoFiles(t, getRepoRoot(t))) {
		violations = append(violations, longLines(file)...)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d production line(s) over %d chars:\n%s",
			len(violations), maxLineLength, strings.Join(violations, "\n"))
	}
}

// blankDiscardAllowlist holds `_ = ident` lines kept on purpose, keyed
// "relative/path.go:line", each with a reason.
var blankDiscardAllowlist = map[string]bool{}

// blankVariableDiscards reports each `_ = ident` in one file. Discarding a call
// result, as in `_ = f()`, is allowed.
func blankVariableDiscards(file goFile) []string {
	var violations []string
	// The closure appends to violations, which ast.Inspect has no way to return.
	ast.Inspect(file.syntax, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lhs, lhsIdent := assign.Lhs[0].(*ast.Ident)
		rhs, rhsIdent := assign.Rhs[0].(*ast.Ident)
		if !lhsIdent || lhs.Name != "_" || !rhsIdent {
			return true
		}
		key := fmt.Sprintf("%s:%d", file.rel, file.fset.Position(assign.Pos()).Line)
		if !blankDiscardAllowlist[key] {
			violations = append(violations, fmt.Sprintf("  %s: `_ = %s` discards a plain variable", key, rhs.Name))
		}
		return true
	})
	return violations
}

// TestNoBlankVariableDiscard flags `_ = x` in production code, where x is a
// plain identifier. It does nothing at runtime except hide a dead variable.
func TestNoBlankVariableDiscard(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, file := range productionFiles(loadGoFiles(t, getRepoRoot(t))) {
		violations = append(violations, blankVariableDiscards(file)...)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d blank-variable discard(s). Delete each one and the dead declaration it "+
			"props up, or allowlist it with a reason:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestHygieneDetectorsFindSeededHits runs the line-length, discard, and marker
// checks against a fixture that breaks each one.
func TestHygieneDetectorsFindSeededHits(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", maxLineLength)
	// Line 16 is under the limit in runes, but over it once its tab counts as tabWidth.
	tabIndented := strings.Repeat("x", maxLineLength-13)
	src := "package sample\n\n" +
		"var a = \"" + long + "\"\n" +
		lineLengthOverrideMarker + "\n" +
		"var b = \"" + long + "\"\n\n" +
		"func f() {\n\tc := 1\n\t_ = c\n\t_ = g()\n}\n\n" +
		"func g() int { return 1 } // " + hackFixmeMarker + "\n\n" +
		"func h() {\n\tprintln(\"" + tabIndented + "\")\n}\n"
	file, err := parseGoSource("sample.go", "sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if got := longLines(file); len(got) != 2 || !strings.HasPrefix(got[0], "  sample.go:3:") ||
		!strings.HasPrefix(got[1], "  sample.go:16:") {
		t.Errorf("longLines = %q, want hits on lines 3 and 16", got)
	}
	if got := blankVariableDiscards(file); len(got) != 1 || !strings.HasPrefix(got[0], "  sample.go:9:") {
		t.Errorf("blankVariableDiscards = %q, want one hit on line 9", got)
	}
	if got := hackFixmeLines(file); len(got) != 1 || !strings.HasPrefix(got[0], "  sample.go:13:") {
		t.Errorf("hackFixmeLines = %q, want one hit on line 13", got)
	}
}

// TestNoTestFilesOutsideTestsDir fails on a _test.go file outside tests/. A
// package-internal test reaches unexported names and couples to the implementation.
func TestNoTestFilesOutsideTestsDir(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, file := range loadGoFiles(t, getRepoRoot(t)) {
		if isTestFile(file.rel) && !inTestsDir(file.rel) {
			violations = append(violations, "  "+file.rel)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d test file(s) outside tests/. Move them under tests/ and drive the code "+
			"through its exported API:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// readOnlyGitSubcommands are the git subcommands a test may run.
var readOnlyGitSubcommands = map[string]bool{
	"ls-files": true, "log": true, "check-ignore": true,
}

// gitWriteReason returns why a call runs git with more than a read-only
// subcommand, or "" for a call that runs no git or only reads.
func gitWriteReason(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
		return ""
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
		return ""
	}
	args := call.Args
	if sel.Sel.Name == "CommandContext" && len(args) > 0 {
		args = args[1:]
	}
	if len(args) == 0 {
		return ""
	}
	if name, ok := stringLiteral(args[0]); !ok || name != "git" {
		return ""
	}
	for i := 1; i < len(args); i++ {
		arg, ok := stringLiteral(args[i])
		switch {
		case !ok:
			return "the git subcommand is not a string literal"
		case arg == "-c" || strings.HasPrefix(arg, "--config-env"):
			return "git " + arg + " can set an alias or a hook"
		case arg == "-C" || arg == "--git-dir" || arg == "--work-tree":
			i++ // the value may be a variable
		case strings.HasPrefix(arg, "-"):
			// a global flag that takes no separate value
		case readOnlyGitSubcommands[arg]:
			return ""
		default:
			return "git " + arg + " is not a read-only subcommand"
		}
	}
	return "the git call names no subcommand"
}

// gitWriteViolations returns "path:line: reason" for each git call in file that can write.
func gitWriteViolations(fset *token.FileSet, file *ast.File, path string) []string {
	var violations []string
	// The closure appends to violations, which ast.Inspect has no way to return.
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if reason := gitWriteReason(call); reason != "" {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", path, fset.Position(call.Pos()).Line, reason))
			}
		}
		return true
	})
	return violations
}

// TestTestsRunOnlyReadOnlyGit fails when a test runs a git subcommand that can
// write. The pre-commit hook runs the suite with git pointed at this repository.
func TestTestsRunOnlyReadOnlyGit(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, file := range loadGoFiles(t, getRepoRoot(t)) {
		if strings.HasPrefix(file.rel, "tests/") {
			violations = append(violations, gitWriteViolations(file.fset, file.syntax, file.rel)...)
		}
	}
	if len(violations) > 0 {
		t.Errorf("tests may run only read-only git (see Git in CONVENTIONS.md):\n%s", strings.Join(violations, "\n"))
	}
}

// TestGitWriteViolationsDetects checks the git scan on calls it must flag and calls it must pass.
func TestGitWriteViolationsDetects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		call string
		want bool
	}{
		{`exec.Command("git", "ls-files", "-z")`, false},
		{`exec.Command("git", "-C", dir, "log", "-1")`, false},
		{`exec.CommandContext(ctx, "git", "check-ignore", "--stdin")`, false},
		{`exec.Command("go", "commit")`, false},
		{`exec.Command("git", "commit", "-m", "x")`, true},
		{`exec.Command("git", "init")`, true},
		{`exec.Command("git", "status")`, true},
		{`exec.CommandContext(ctx, "git", "config", "user.name", "x")`, true},
		{`exec.Command("git", "-c", "core.hooksPath=x", "status")`, true},
		{`exec.Command("git", "--config-env=core.hooksPath=X", "status")`, true},
		{`exec.Command("git", args...)`, true},
		{`exec.Command("git", sub, "x")`, true},
		{`exec.Command("git")`, true},
	}
	for _, tc := range cases {
		src := "package p\nfunc f() { _ = " + tc.call + " }\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.call, err)
		}
		if got := len(gitWriteViolations(fset, file, "fixture.go")) > 0; got != tc.want {
			t.Errorf("%s: flagged = %v, want %v", tc.call, got, tc.want)
		}
	}
}

// dumpIgnoreCases are paths, and whether .gitignore must keep `git add -A` from
// staging each. The negatives catch a pattern broad enough to hide source.
var dumpIgnoreCases = []struct {
	path        string
	wantIgnored bool
	why         string
}{
	{"app.dump", true, "the usual pg_dump target"},
	{"app.dump.gz", true, "a compressed dump"},
	{"backups/nightly.dump", true, "a dump in a subdirectory"},
	{"app.dump.1", true, "a rotated dump"},
	{"app.dump.gz.part", true, "an interrupted transfer"},
	{"app.sql.gz", true, "plain pg_dump output piped through gzip"},
	{"app.sql.zst", true, "the same, through zstd"},
	{"db.pgdump", true, "pg_dump -Fc, .pgdump convention"},
	{"db.backup", true, "pg_dump -Fc, .backup convention"},
	{"pg_dump_2026.custom", true, "pg_dump -Fc, .custom convention"},
	{"local.db", true, "a SQLite copy of the data"},
	{"scratch.sqlite3", true, "SQLite under its other suffix"},
	{"orders_export.tar.gz", true, "a bulk export archived for transfer"},

	{"main.go", false, "source stays committable"},
	{"create_tables.sql", false, "the schema is tracked, so bare .sql stays committable"},
	{"migrations/001_create_orders.sql", false, "migrations are tracked"},
	{"README.md", false, "docs are tracked"},
	{"internal/repository/order_repo.go", false, "nested source"},
}

// TestGitignoreCoversDatabaseDumps runs each fixture path through `git
// check-ignore --no-index`, which answers from the ignore rules alone.
func TestGitignoreCoversDatabaseDumps(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	requireGitWorkTree(t, root)

	var stdin bytes.Buffer
	for _, tc := range dumpIgnoreCases {
		stdin.WriteString(tc.path)
		stdin.WriteByte(0)
	}
	cmd := exec.Command("git", "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Dir = root
	cmd.Stdin = &stdin
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	// Exit 1 means none of the paths are ignored, which is an answer, not a failure.
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		t.Fatalf("git check-ignore: %v", err)
	}

	ignored := map[string]bool{}
	for _, path := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		ignored[path] = true
	}
	for _, tc := range dumpIgnoreCases {
		switch got := ignored[tc.path]; {
		case got == tc.wantIgnored:
		case tc.wantIgnored:
			t.Errorf("%q is not gitignored (%s). Add a covering pattern to .gitignore", tc.path, tc.why)
		default:
			t.Errorf("%q is gitignored (%s). A dump pattern is too broad", tc.path, tc.why)
		}
	}
}

// trackedDumpShapes matches paths shaped like a database dump or bulk archive. It
// is written apart from .gitignore, so a wrong ignore rule does not hide here too.
var trackedDumpShapes = regexp.MustCompile(
	`(?i)(\.dump($|\.)|\.dmp$|\.pgdump$|\.backup$|\.custom$|\.db$|\.sqlite3?$|` +
		`\.sql\.(gz|bz2|xz|zst)$|\.tar\.gz$|\.tgz$|\.zip$)`)

// TestNoDatabaseDumpsTracked fails on a tracked file shaped like a dump. It
// catches one the ignore rules miss, such as a file added with `git add -f`.
func TestNoDatabaseDumpsTracked(t *testing.T) {
	t.Parallel()
	var violations []string
	for _, rel := range trackedFiles(t, getRepoRoot(t)) {
		if trackedDumpShapes.MatchString(rel) {
			violations = append(violations, "  "+rel)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d tracked file(s) shaped like a database dump or bulk archive. Remove "+
			"them from git; a dump in history stays readable to every clone:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// TestTrackedDumpShapesDetects checks the dump pattern against the ignore fixtures.
func TestTrackedDumpShapesDetects(t *testing.T) {
	t.Parallel()
	for _, tc := range dumpIgnoreCases {
		if got := trackedDumpShapes.MatchString(tc.path); got != tc.wantIgnored {
			t.Errorf("%q: matched = %v, want %v (%s)", tc.path, got, tc.wantIgnored, tc.why)
		}
	}
}

// negatedAssignmentGuard matches a guard that negates an assignment from a
// command substitution holding a pipeline.
var negatedAssignmentGuard = regexp.MustCompile(`if\s+!\s+[A-Za-z_][A-Za-z0-9_]*="\$\([^)]*\|[^)]*\)"`)

// pipefailSetting matches a set line that turns pipefail on, alone or with other flags.
var pipefailSetting = regexp.MustCompile(`(?m)^\s*set\s+-[a-zA-Z]*o\s+pipefail\b`)

// shellScriptPrologue matches a shell shebang, so an extensionless script is found too.
var shellScriptPrologue = regexp.MustCompile(`^#!.*\b(ba|da|k|z)?sh\b`)

// hasShellShebang reports whether a file's first line is a shell shebang. It
// reads one buffer at most, so a large binary costs no more than a script.
func hasShellShebang(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	firstLine, err := bufio.NewReader(file).ReadSlice('\n')
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return false, err
	}
	return shellScriptPrologue.Match(firstLine), nil
}

// shellScripts returns every shell script under root, outside the skipped trees.
func shellScripts(t *testing.T, root string) map[string][]byte {
	t.Helper()
	scripts := map[string][]byte{}
	// The closure adds to scripts, which walkScanTree has no way to return.
	err := walkScanTree(root, func(path, rel string) error {
		isScript := strings.HasSuffix(path, ".sh")
		if !isScript {
			shebang, shebangErr := hasShellShebang(path)
			if shebangErr != nil {
				return shebangErr
			}
			isScript = shebang
		}
		if !isScript {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scripts[rel] = src
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return scripts
}

// negatedPipelineGuards reports each line of a script that negates a pipeline
// assignment while pipefail is off. Such a guard sees only the last stage's status.
func negatedPipelineGuards(rel string, src []byte) []string {
	if pipefailSetting.Match(src) {
		return nil
	}
	var violations []string
	for i, line := range strings.Split(string(src), "\n") {
		if negatedAssignmentGuard.MatchString(line) {
			violations = append(violations, fmt.Sprintf("  %s:%d: %s", rel, i+1, strings.TrimSpace(line)))
		}
	}
	return violations
}

// TestNegatedAssignmentGuardsSetPipefail fails on a shell script that negates
// an assignment from a pipeline without setting pipefail.
func TestNegatedAssignmentGuardsSetPipefail(t *testing.T) {
	t.Parallel()
	var violations []string
	for rel, src := range shellScripts(t, getRepoRoot(t)) {
		violations = append(violations, negatedPipelineGuards(rel, src)...)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d negated pipeline assignment(s) without `set -o pipefail`. An early "+
			"stage's failure leaves the status at zero:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestNegatedPipelineGuardsDetects checks the pipefail scan on a script that
// needs it, and on the same script once it sets it.
func TestNegatedPipelineGuardsDetects(t *testing.T) {
	t.Parallel()
	guard := `if ! rows="$(psql -tAc 'SELECT 1' | head -1)"; then exit 1; fi` + "\n"
	if got := negatedPipelineGuards("a.sh", []byte("#!/usr/bin/env bash\n"+guard)); len(got) != 1 {
		t.Errorf("without pipefail: got %q, want one hit", got)
	}
	if got := negatedPipelineGuards("b.sh", []byte("#!/usr/bin/env bash\nset -euo pipefail\n"+guard)); len(got) != 0 {
		t.Errorf("with pipefail: got %q, want none", got)
	}
}
