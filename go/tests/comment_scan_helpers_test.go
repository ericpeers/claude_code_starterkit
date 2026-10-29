// SPDX-License-Identifier: MIT

package tests

// Prose extraction for the style gates in comment_style_test.go. One scanner per
// file type returns what a person wrote, with the comment marker attached.

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// proseLine is one line of comment or documentation, at its 1-indexed position.
type proseLine struct {
	num  int
	text string
}

// proseFile is one scanned file, read once and shared by every guard. A Go
// file also keeps its parse, so loadGoFiles does not parse it again.
type proseFile struct {
	rel    string
	path   string
	lines  []string
	prose  []proseLine
	fset   *token.FileSet
	syntax *ast.File
}

// proseScan is one walk of the tree: the files read, and the ones whose Go
// source did not parse.
type proseScan struct {
	files   []proseFile
	skipped []string
}

var (
	proseOnce sync.Once
	proseTree proseScan
	proseErr  error
)

// loadProseFiles returns the scanned tree, walking and parsing it on the first
// call. Every gate runs in parallel and shares that one pass.
func loadProseFiles(t *testing.T, root string) []proseFile {
	t.Helper()

	// Once.Do takes no arguments, so the closure carries root.
	proseOnce.Do(func() { proseTree, proseErr = readProseFiles(root) })
	if proseErr != nil {
		t.Fatalf("scanning the source tree: %v", proseErr)
	}
	for _, skipped := range proseTree.skipped {
		t.Logf("comment-style gates skipped a file: %s", skipped)
	}
	return proseTree.files
}

// scanSkipDirs are trees the gates do not read. They hold vendored code, build
// output, virtualenvs, and scratch notes.
var scanSkipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "build": true,
	"dist": true, "venv": true, ".venv": true, "__pycache__": true,
	"notes": true, "migration_out": true,
}

// scanSkipFiles are repo-relative paths the gates do not read: swag output and
// the author's backlog.
var scanSkipFiles = map[string]bool{
	"docs/docs.go": true,
	"todo.md":      true,
}

// inScope reports whether the gates read one file, given its repo-relative path
// in slash form.
func inScope(rel string) bool {
	if scanSkipFiles[rel] {
		return false
	}
	switch filepath.Ext(rel) {
	case ".go", ".md", ".py":
		return true
	case ".sql":
		return !strings.HasPrefix(rel, "migrations/")
	}
	return false
}

// errUnparsed marks source the Go parser rejected. The walk records the file
// and moves on, so one bad file leaves the other gates running.
var errUnparsed = errors.New("source does not parse")

// collectProseFile reads and scans one file. A parse failure is wrapped in
// errUnparsed, so the walk can tell it from an unreadable tree.
func collectProseFile(path, rel string) (proseFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return proseFile{}, err
	}
	file, scanErr := scanSource(rel, path, data)
	if scanErr != nil {
		return proseFile{}, fmt.Errorf("%w: %v", errUnparsed, scanErr)
	}
	return file, nil
}

// scanSource scans one file's source with the scanner for its extension. It
// reports an error only when Go source does not parse.
func scanSource(rel, path string, data []byte) (proseFile, error) {
	file := proseFile{rel: rel, path: path, lines: strings.Split(string(data), "\n")}
	switch filepath.Ext(path) {
	case ".go":
		file.fset = token.NewFileSet()
		syntax, err := parser.ParseFile(file.fset, path, data, parser.ParseComments)
		if err != nil {
			return proseFile{}, fmt.Errorf("parsing %s: %w", path, err)
		}
		file.syntax = syntax
		file.prose = scanGoProse(file.fset, file.syntax)
	case ".md":
		file.prose = scanMarkdownProse(string(data))
	case ".sql":
		file.prose = scanSQLProse(string(data))
	case ".py":
		file.prose = scanPythonProse(string(data))
	}
	return file, nil
}

// walkScanTree calls visit on each file under root, outside scanSkipDirs. rel
// is repo-relative, in slash form.
func walkScanTree(root string, visit func(path, rel string) error) error {
	// WalkDir takes a func value, and this one forwards to visit.
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if scanSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		return visit(path, filepath.ToSlash(relFromRoot(root, path)))
	})
}

// readProseFiles walks the tree rooted at root and scans every file in scope.
// root is the Go module the gates run in, which is the whole repository only
// when go.mod sits at its top.
func readProseFiles(root string) (proseScan, error) {
	var scan proseScan

	// The closure appends to scan, which walkScanTree has no way to return.
	err := walkScanTree(root, func(path, rel string) error {
		if !inScope(rel) {
			return nil
		}
		file, collectErr := collectProseFile(path, rel)
		if errors.Is(collectErr, errUnparsed) {
			scan.skipped = append(scan.skipped, collectErr.Error())
			return nil
		}
		if collectErr != nil {
			return collectErr
		}
		scan.files = append(scan.files, file)
		return nil
	})
	if err != nil {
		return proseScan{}, err
	}
	return scan, nil
}

// relFromRoot renders path relative to root, falling back to path itself.
func relFromRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// goDirective matches the comment forms a tool reads rather than a person: build
// and generate directives, and the swag annotations that become swagger.json.
var goDirective = regexp.MustCompile(`^//(go:|\s*[@+]|line )`)

// isLicenseHeader reports whether a comment group is the license notice: it
// opens on line 1, above the package clause.
func isLicenseHeader(fset *token.FileSet, file *ast.File, group *ast.CommentGroup) bool {
	return fset.Position(group.Pos()).Line == 1 && group.Pos() < file.Package
}

// scanGoProse returns the comment lines of a parsed Go file. The license header
// and tool directives are left out.
func scanGoProse(fset *token.FileSet, parsed *ast.File) []proseLine {
	var out []proseLine
	for _, group := range parsed.Comments {
		if isLicenseHeader(fset, parsed, group) {
			continue
		}
		for _, comment := range group.List {
			if goDirective.MatchString(comment.Text) {
				continue
			}
			first := fset.Position(comment.Pos()).Line
			for offset, line := range strings.Split(comment.Text, "\n") {
				out = append(out, proseLine{first + offset, line})
			}
		}
	}
	return out
}

// markdownInlineCode matches a backtick span, which holds identifiers and
// commands rather than prose.
var markdownInlineCode = regexp.MustCompile("`[^`]*`")

// scanMarkdownProse returns the prose lines of a Markdown file. Fenced blocks
// are dropped whole and inline backtick spans are removed from what is left.
func scanMarkdownProse(data string) []proseLine {
	var out []proseLine
	fenced := false
	for i, line := range strings.Split(data, "\n") {
		opener := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(opener, "```") || strings.HasPrefix(opener, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		text := markdownInlineCode.ReplaceAllString(line, "")
		if strings.TrimSpace(text) != "" {
			out = append(out, proseLine{i + 1, text})
		}
	}
	return out
}

// scanSQLProse returns a SQL file's `--` and `/* */` comment text. Single-quoted
// literals are stepped over, so a `--` inside a string is not read.
func scanSQLProse(data string) []proseLine {
	var out []proseLine
	inBlock := false
	for i, line := range strings.Split(data, "\n") {
		var comment strings.Builder
		inString := false
		for pos := 0; pos < len(line); pos++ {
			rest := line[pos:]
			switch {
			case inBlock:
				if strings.HasPrefix(rest, "*/") {
					inBlock = false
					pos++
					continue
				}
				comment.WriteByte(line[pos])
			case inString:
				if line[pos] == '\'' {
					inString = false
				}
			case line[pos] == '\'':
				inString = true
			case strings.HasPrefix(rest, "--"):
				comment.WriteString(rest)
				pos = len(line)
			case strings.HasPrefix(rest, "/*"):
				inBlock = true
				pos++
			}
		}
		if text := comment.String(); strings.TrimSpace(text) != "" {
			out = append(out, proseLine{i + 1, text})
		}
	}
	return out
}

// pythonTripleQuotes are the two docstring delimiters.
var pythonTripleQuotes = []string{`"""`, `'''`}

// pythonOpen is an unterminated triple-quoted string at the end of a line. prose
// is true when the string opened the line, which makes it a docstring.
type pythonOpen struct {
	delim string
	prose bool
}

// pythonLineProse reads one Python line that starts outside a docstring. It
// returns the prose on it, and any triple quote the line leaves open. A triple
// quote with code before it opens a string literal, whose body is not prose.
func pythonLineProse(line string) (text string, open pythonOpen) {
	quote := byte(0)
	for pos := 0; pos < len(line); pos++ {
		rest := line[pos:]
		if quote != 0 {
			switch line[pos] {
			case '\\':
				pos++
			case quote:
				quote = 0
			}
			continue
		}
		if delim := tripleQuoteAt(rest); delim != "" {
			docstring := strings.TrimSpace(line[:pos]) == ""
			end := strings.Index(rest[len(delim):], delim)
			if end < 0 {
				return docstringText(line, docstring), pythonOpen{delim, docstring}
			}
			if docstring {
				return line, pythonOpen{}
			}
			// Step past the closing delimiter, so a trailing # is still read.
			pos += 2*len(delim) + end - 1
			continue
		}
		switch line[pos] {
		case '\'', '"':
			quote = line[pos]
		case '#':
			return rest, pythonOpen{}
		}
	}
	return "", pythonOpen{}
}

// docstringText returns the line when it opens a docstring, and nothing when it
// opens a string literal in code.
func docstringText(line string, docstring bool) string {
	if docstring {
		return line
	}
	return ""
}

// tripleQuoteAt returns the docstring delimiter opening rest, or an empty string.
func tripleQuoteAt(rest string) string {
	for _, delim := range pythonTripleQuotes {
		if strings.HasPrefix(rest, delim) {
			return delim
		}
	}
	return ""
}

// scanPythonProse returns the `#` comments and docstring lines of a Python file.
// Quoted strings are tracked, so a `#` inside a literal stays out of the result.
func scanPythonProse(data string) []proseLine {
	var out []proseLine
	var open pythonOpen
	for i, line := range strings.Split(data, "\n") {
		if open.delim != "" {
			if open.prose {
				out = append(out, proseLine{i + 1, line})
			}
			if strings.Contains(line, open.delim) {
				open = pythonOpen{}
			}
			continue
		}
		text, next := pythonLineProse(line)
		if text != "" {
			out = append(out, proseLine{i + 1, text})
		}
		open = next
	}
	return out
}
