// SPDX-License-Identifier: MIT

package tests

// Shared helpers for the gates that read Go source or run Go tools.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// goFile is one parsed Go source file. rel is repo-relative, in slash form.
type goFile struct {
	rel    string
	path   string
	lines  []string
	fset   *token.FileSet
	syntax *ast.File
}

var (
	goFilesOnce sync.Once
	goFilesTree []goFile
)

// loadGoFiles returns every Go file the comment-style gates read, parsed with
// comments. Generated files are left out. The gates share one parse.
func loadGoFiles(t *testing.T, root string) []goFile {
	t.Helper()
	files := loadProseFiles(t, root)
	// Once.Do takes no arguments, so the closure carries files.
	goFilesOnce.Do(func() { goFilesTree = goFilesFromProse(files) })
	return goFilesTree
}

// goFilesFromProse keeps the Go files among the scanned files, reusing the
// parse the prose scan made.
func goFilesFromProse(files []proseFile) []goFile {
	var out []goFile
	for _, file := range files {
		if file.syntax == nil || ast.IsGenerated(file.syntax) {
			continue
		}
		out = append(out, goFile{rel: file.rel, path: file.path, lines: file.lines, fset: file.fset, syntax: file.syntax})
	}
	return out
}

// parseGoSource parses one file's source.
func parseGoSource(rel, path, src string) (goFile, error) {
	fset := token.NewFileSet()
	syntax, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return goFile{}, fmt.Errorf("parsing %s: %w", rel, err)
	}
	return goFile{rel: rel, path: path, lines: strings.Split(src, "\n"), fset: fset, syntax: syntax}, nil
}

// isTestFile reports whether a repo-relative path names a Go test file.
func isTestFile(rel string) bool {
	return strings.HasSuffix(rel, "_test.go")
}

// inTestsDir reports whether a repo-relative path sits under tests/.
func inTestsDir(rel string) bool {
	return strings.HasPrefix(rel, "tests/")
}

// productionGoFile reports whether a file is application source: not a test
// file, and outside tests/.
func productionGoFile(rel string) bool {
	return !isTestFile(rel) && !inTestsDir(rel)
}

// productionFiles keeps the files outside tests/.
func productionFiles(files []goFile) []goFile {
	var out []goFile
	for _, file := range files {
		if productionGoFile(file.rel) {
			out = append(out, file)
		}
	}
	return out
}

// goTool returns a command that runs a tool declared in go.mod, at the version
// go.mod names.
func goTool(name string, args ...string) *exec.Cmd {
	return exec.Command("go", append([]string{"tool", name}, args...)...)
}

// goOutput runs the go command in root and returns its stdout. A failure ends
// the test with stderr.
func goOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return output
}

// goList runs go list in root and returns the fields it prints.
func goList(t *testing.T, root string, args ...string) []string {
	t.Helper()
	return strings.Fields(string(goOutput(t, root, append([]string{"list"}, args...)...)))
}

// loadTypedPackages type-checks the packages matching patterns from root.
// withTests adds each package's test variants.
func loadTypedPackages(t *testing.T, root string, withTests bool, patterns ...string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports,
		Dir:   root,
		Tests: withTests,
	}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		t.Fatalf("loading %v: %v", patterns, err)
	}
	if failed := packages.PrintErrors(loaded); failed > 0 {
		t.Fatalf("%d package(s) in %v failed to load; the scan would see no types", failed, patterns)
	}
	return loaded
}

// optionalDir holds opt-in modules that install.sh copies into internal/. Its
// packages are not wired into a main package, so reachability gates leave it out.
const optionalDir = "optional"

// modulePath returns the module path go.mod declares.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	fields := goList(t, root, "-m")
	if len(fields) != 1 {
		t.Fatalf("go list -m printed %q, want one module path", fields)
	}
	return fields[0]
}

// mainPackages returns the import path of every main package outside optional/.
// A fresh scaffold has none until main.go is added.
func mainPackages(t *testing.T, root string) []string {
	t.Helper()
	optional := modulePath(t, root) + "/" + optionalDir + "/"
	var mains []string
	for _, pkg := range goList(t, root, "-f", `{{if eq .Name "main"}}{{.ImportPath}}{{end}}`, "./...") {
		if !strings.HasPrefix(pkg, optional) {
			mains = append(mains, pkg)
		}
	}
	return mains
}

// stringLiteral returns the value of a plain string literal.
func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}
