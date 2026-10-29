// SPDX-License-Identifier: MIT

package tests

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// helperFileSuffix is the ending every topic helper file carries. A file named
// this way holds shared fixtures for its topic and no test functions of its own.
const helperFileSuffix = "_helpers_test.go"

// helperFilePattern matches a topic helper file name, such as
// order_helpers_test.go. The topic is lowercase words joined by underscores.
var helperFilePattern = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*` + regexp.QuoteMeta(helperFileSuffix) + `$`)

// suiteSetupFile holds TestMain and the fixtures the whole suite draws on.
const suiteSetupFile = "setup_test.go"

// declaresSharedHelpers reports whether a file may declare identifiers that
// other test files use.
func declaresSharedHelpers(base string) bool {
	return base == suiteSetupFile || helperFilePattern.MatchString(base)
}

// suggestedHelperFile renders the helper file a test file's shared identifiers
// belong in. It keeps the topic and swaps the suffix.
func suggestedHelperFile(base string) string {
	return strings.TrimSuffix(base, "_test.go") + helperFileSuffix
}

// sharedHelper is one package-level identifier used outside the file declaring it.
type sharedHelper struct {
	name  string
	users int
}

// crossFileHelpers groups every shared identifier by the file that declares it.
// A test function is skipped, since the runner is its only caller.
func crossFileHelpers(t *testing.T, pkg *packages.Package) map[string][]sharedHelper {
	t.Helper()

	fileOfObject := map[types.Object]string{}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		object := scope.Lookup(name)
		fileOfObject[object] = filepath.Base(pkg.Fset.Position(object.Pos()).Filename)
	}

	userFiles := map[types.Object]map[string]bool{}
	for ident, object := range pkg.TypesInfo.Uses {
		declFile, tracked := fileOfObject[object]
		if !tracked || isTestEntryPoint(object) {
			continue
		}
		useFile := filepath.Base(pkg.Fset.Position(ident.Pos()).Filename)
		if useFile == declFile {
			continue
		}
		if userFiles[object] == nil {
			userFiles[object] = map[string]bool{}
		}
		userFiles[object][useFile] = true
	}

	byFile := map[string][]sharedHelper{}
	for object, users := range userFiles {
		declFile := fileOfObject[object]
		if declaresSharedHelpers(declFile) {
			continue
		}
		byFile[declFile] = append(byFile[declFile], sharedHelper{name: object.Name(), users: len(users)})
	}
	for _, helpers := range byFile {
		slices.SortFunc(helpers, compareHelperName)
	}
	return byFile
}

// compareHelperName orders shared helpers by name.
func compareHelperName(first, second sharedHelper) int {
	return cmp.Compare(first.name, second.name)
}

// isTestEntryPoint reports whether an object is a function the go test runner calls.
// The signature is checked as well as the name, so a shared fixture named TestFixture
// stays in the scan.
func isTestEntryPoint(object types.Object) bool {
	function, isFunction := object.(*types.Func)
	if !isFunction {
		return false
	}
	signature, isSignature := function.Type().(*types.Signature)
	if !isSignature || signature.Recv() != nil || signature.Results().Len() != 0 {
		return false
	}
	name := function.Name()
	if strings.HasPrefix(name, "Example") {
		return signature.Params().Len() == 0
	}
	if signature.Params().Len() != 1 {
		return false
	}
	param := signature.Params().At(0).Type().String()
	switch {
	case strings.HasPrefix(name, "Test"):
		return param == "*testing.T" || param == "*testing.M"
	case strings.HasPrefix(name, "Benchmark"):
		return param == "*testing.B"
	case strings.HasPrefix(name, "Fuzz"):
		return param == "*testing.F"
	}
	return false
}

// TestSharedHelpersLiveInHelperFiles enforces one home for shared test fixtures.
// A package-level identifier read from a second file belongs in setup_test.go or
// in a topic file ending in _helpers_test.go. Every file in tests/ shares one package.
func TestSharedHelpersLiveInHelperFiles(t *testing.T) {
	t.Parallel()

	byFile := crossFileHelpers(t, typeCheckTestPackage(t))
	if len(byFile) == 0 {
		return
	}

	files := make([]string, 0, len(byFile))
	for declFile := range byFile {
		files = append(files, declFile)
	}
	// The comparator reads byFile, which a named function would need passed in.
	slices.SortFunc(files, func(first, second string) int {
		if byCount := cmp.Compare(len(byFile[second]), len(byFile[first])); byCount != 0 {
			return byCount
		}
		return cmp.Compare(first, second)
	})

	total := 0
	var report []string
	for _, declFile := range files {
		helpers := byFile[declFile]
		total += len(helpers)
		names := make([]string, 0, len(helpers))
		for _, helper := range helpers {
			names = append(names, fmt.Sprintf("%s (used by %d)", helper.name, helper.users))
		}
		report = append(report, fmt.Sprintf("  %s -> %s\n      %s",
			declFile, suggestedHelperFile(declFile), strings.Join(names, ", ")))
	}

	t.Errorf("%d identifiers in %d files are used from another test file.\n"+
		"Move each into the helper file named on the right, or into %s when the whole suite draws on it.\n%s",
		total, len(files), suiteSetupFile, strings.Join(report, "\n"))
}

// typeCheckTestPackage type-checks this package and returns the variant that
// holds the test files.
func typeCheckTestPackage(t *testing.T) *packages.Package {
	t.Helper()
	// A test-only directory loads as several variants. The one with the test
	// files has the most syntax trees.
	var pkg *packages.Package
	for _, candidate := range loadTypedPackages(t, getRepoRoot(t), true, "./tests") {
		if pkg == nil || len(candidate.Syntax) > len(pkg.Syntax) {
			pkg = candidate
		}
	}
	if pkg == nil || len(pkg.Syntax) == 0 {
		t.Fatal("no test files were type-checked")
	}
	return pkg
}

// TestIsTestEntryPoint fails when the runner's entry points are confused with
// shared fixtures that only share their prefix.
func TestIsTestEntryPoint(t *testing.T) {
	t.Parallel()
	const src = `package sample

import "testing"

func TestRuns(t *testing.T)        {}
func TestMain(m *testing.M)        {}
func BenchmarkRuns(b *testing.B)   {}
func FuzzRuns(f *testing.F)        {}
func ExampleRuns()                 {}
func TestFixture() string          { return "" }
func TestWithResult(t *testing.T) int { return 0 }
func ExampleWithArg(n int)         {}
func BenchmarkWrongParam(t *testing.T) {}

type suite struct{}

func (suite) TestMethod(t *testing.T) {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample_test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	config := types.Config{Importer: importer.Default()}
	if _, err := config.Check("sample", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"TestRuns": true, "TestMain": true, "BenchmarkRuns": true, "FuzzRuns": true, "ExampleRuns": true,
		"TestFixture": false, "TestWithResult": false, "ExampleWithArg": false, "BenchmarkWrongParam": false,
		"TestMethod": false,
	}
	funcsByName := map[string]types.Object{}
	for ident, object := range info.Defs {
		if _, isFunc := object.(*types.Func); isFunc {
			funcsByName[ident.Name] = object
		}
	}
	for name, expected := range want {
		object, found := funcsByName[name]
		if !found {
			t.Errorf("fixture declares no function %s", name)
		} else if isTestEntryPoint(object) != expected {
			t.Errorf("isTestEntryPoint(%s) = %v, want %v", name, !expected, expected)
		}
	}
}
