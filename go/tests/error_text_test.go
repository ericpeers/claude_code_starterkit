// SPDX-License-Identifier: MIT

package tests

// Gate that keeps error text out of HTTP responses.

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// errorTextFixtureDir is the self-test package. It sits under testdata, so
// `./...` leaves it out of the build and of the static-analysis gates.
const errorTextFixtureDir = "tests/testdata/errortext"

// ginPackage is the import path of the gin web framework.
const ginPackage = "github.com/gin-gonic/gin"

// replyMethods are the *gin.Context methods that write a response body from a Go value.
var replyMethods = map[string]bool{
	"JSON": true, "AbortWithStatusJSON": true, "IndentedJSON": true,
	"PureJSON": true, "SecureJSON": true, "AsciiJSON": true, "JSONP": true,
	"String": true, "Data": true, "XML": true, "YAML": true, "TOML": true,
	"ProtoBuf": true, "SSEvent": true, "Negotiate": true,
}

// isGinReply reports whether a selector calls a reply method on a *gin.Context.
func isGinReply(pkg *packages.Package, sel *ast.SelectorExpr) bool {
	if !replyMethods[sel.Sel.Name] {
		return false
	}
	selection, ok := pkg.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return false
	}
	recv := selection.Recv()
	if ptr, isPtr := recv.(*types.Pointer); isPtr {
		recv = ptr.Elem()
	}
	named, ok := types.Unalias(recv).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == ginPackage && obj.Name() == "Context"
}

// funcSite is one function declaration and the package it was type-checked in.
type funcSite struct {
	pkg  *packages.Package
	decl *ast.FuncDecl
}

// funcObj returns the declared function.
func (f funcSite) funcObj() *types.Func {
	fn, _ := f.pkg.TypesInfo.Defs[f.decl.Name].(*types.Func)
	return fn
}

// callSite is one call and the function it sits in.
type callSite struct {
	funcSite
	call *ast.CallExpr
}

// visit is one object followed on behalf of one reply sink.
type visit struct {
	obj  types.Object
	sink string
}

// errorTextScan traces reply text back to its inputs across packages. It follows
// locals, parameters through their callers, and the returns of module functions.
// seen is keyed per sink, so two replies that share a helper are both reported.
type errorTextScan struct {
	root       string
	errorIface *types.Interface
	decls      map[*types.Func]funcSite
	calls      map[*types.Func][]callSite
	seen       map[visit]bool
	violations map[string]bool
}

// newErrorTextScan indexes every function declaration and call in pkgs.
func newErrorTextScan(root string, pkgs []*packages.Package) *errorTextScan {
	s := &errorTextScan{
		root:       root,
		errorIface: types.Universe.Lookup("error").Type().Underlying().(*types.Interface),
		decls:      map[*types.Func]funcSite{},
		calls:      map[*types.Func][]callSite{},
		seen:       map[visit]bool{},
		violations: map[string]bool{},
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, fileDecl := range file.Decls {
				if decl, ok := fileDecl.(*ast.FuncDecl); ok && decl.Body != nil {
					s.index(funcSite{pkg: pkg, decl: decl})
				}
			}
		}
	}
	return s
}

// index records one declaration and every call in its body.
func (s *errorTextScan) index(site funcSite) {
	if fn := site.funcObj(); fn != nil {
		s.decls[fn] = site
	}
	// The closure reads site and adds to s.calls.
	ast.Inspect(site.decl.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if fn := calleeFunc(site.pkg, call); fn != nil {
				s.calls[fn] = append(s.calls[fn], callSite{funcSite: site, call: call})
			}
		}
		return true
	})
}

// scanReplies checks every argument after the status code of each reply call.
func (s *errorTextScan) scanReplies() {
	for _, site := range s.decls {
		// The closure reads site and adds to s.violations.
		ast.Inspect(site.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !isGinReply(site.pkg, sel) || len(call.Args) < 2 {
				return true
			}
			sink := s.where(site.pkg, call.Pos())
			for _, arg := range call.Args[1:] {
				s.check(site, arg, sink)
			}
			return true
		})
	}
}

// where formats a position relative to the module root.
func (s *errorTextScan) where(pkg *packages.Package, pos token.Pos) string {
	position := pkg.Fset.Position(pos)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(relFromRoot(s.root, position.Filename)), position.Line)
}

// check reports error text in expr, then follows the variables and calls that feed it.
func (s *errorTextScan) check(site funcSite, expr ast.Expr, sink string) {
	if pos, found := errorTextIn(site.pkg, s.errorIface, expr); found {
		s.violations[fmt.Sprintf("%s: error text, reaching %s", s.where(site.pkg, pos), sink)] = true
		return
	}
	// The closure reads site and sink to follow each name and call.
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.Ident:
			if textVar, ok := site.pkg.TypesInfo.Uses[n].(*types.Var); ok && !textVar.IsField() && carriesText(textVar.Type()) {
				s.followVar(site, textVar, sink)
			}
		case *ast.CallExpr:
			if fn := calleeFunc(site.pkg, n); fn != nil {
				s.followReturns(fn, sink)
			}
		}
		return true
	})
}

// followVar checks what is assigned to textVar, or what callers pass when it is a parameter.
func (s *errorTextScan) followVar(site funcSite, textVar *types.Var, sink string) {
	if s.seen[visit{textVar, sink}] {
		return
	}
	s.seen[visit{textVar, sink}] = true
	if idx, variadic, ok := paramIndex(site, textVar); ok {
		for _, caller := range s.calls[site.funcObj()] {
			for i, arg := range caller.call.Args {
				if i == idx || (variadic && i > idx) {
					s.check(caller.funcSite, arg, sink)
				}
			}
		}
		return
	}
	// The closure reads site, textVar, and sink to check each assignment to textVar.
	ast.Inspect(site.decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if !namesVar(site.pkg, textVar, lhs) {
					continue
				}
				if len(n.Rhs) == len(n.Lhs) {
					s.check(site, n.Rhs[i], sink)
				} else {
					s.check(site, n.Rhs[0], sink)
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if namesVar(site.pkg, textVar, name) && i < len(n.Values) {
					s.check(site, n.Values[i], sink)
				}
			}
		case *ast.RangeStmt:
			if (n.Key != nil && namesVar(site.pkg, textVar, n.Key)) || (n.Value != nil && namesVar(site.pkg, textVar, n.Value)) {
				s.check(site, n.X, sink)
			}
		}
		return true
	})
}

// followReturns checks the text values fn returns, when fn is declared in the scan.
func (s *errorTextScan) followReturns(fn *types.Func, sink string) {
	if s.seen[visit{fn, sink}] {
		return
	}
	s.seen[visit{fn, sink}] = true
	site, ok := s.decls[fn]
	if !ok {
		return
	}
	// The closure reads site and sink to check each returned text value.
	ast.Inspect(site.decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			for _, result := range n.Results {
				if carriesText(site.pkg.TypesInfo.TypeOf(result)) {
					s.check(site, result, sink)
				}
			}
		}
		return true
	})
}

// sortedViolations returns the findings in order.
func (s *errorTextScan) sortedViolations() []string {
	lines := make([]string, 0, len(s.violations))
	for violation := range s.violations {
		lines = append(lines, "  "+violation)
	}
	sort.Strings(lines)
	return lines
}

// namesVar reports whether expr is an identifier for textVar.
func namesVar(pkg *packages.Package, textVar *types.Var, expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && (pkg.TypesInfo.Defs[ident] == textVar || pkg.TypesInfo.Uses[ident] == textVar)
}

// paramIndex reports textVar's position among site's parameters, and whether it is the variadic one.
func paramIndex(site funcSite, textVar *types.Var) (idx int, variadic bool, found bool) {
	fn := site.funcObj()
	if fn == nil {
		return 0, false, false
	}
	sig := fn.Signature()
	for i := range sig.Params().Len() {
		if sig.Params().At(i) == textVar {
			return i, sig.Variadic() && i == sig.Params().Len()-1, true
		}
	}
	return 0, false, false
}

// calleeFunc returns the function a call names, or nil for a call through a value.
func calleeFunc(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	case *ast.IndexExpr:
		ident, _ = fun.X.(*ast.Ident)
	case *ast.IndexListExpr:
		ident, _ = fun.X.(*ast.Ident)
	}
	if ident == nil {
		return nil
	}
	fn, ok := pkg.TypesInfo.Uses[ident].(*types.Func)
	if !ok {
		return nil
	}
	return fn.Origin()
}

// carriesText reports whether t is a string or a slice of strings.
func carriesText(t types.Type) bool {
	if t == nil {
		return false
	}
	if slice, ok := t.Underlying().(*types.Slice); ok {
		t = slice.Elem()
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

// errorTextIn reports where node formats an error or calls its Error method.
// Reading a field of a typed error is allowed.
func errorTextIn(pkg *packages.Package, errorIface *types.Interface, node ast.Node) (token.Pos, bool) {
	var pos token.Pos
	found := false
	// The closure sets pos and found at the first error text.
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if sel, ok := expr.(*ast.SelectorExpr); ok && isErrorExpr(pkg, errorIface, sel.X) && !isErrorExpr(pkg, errorIface, sel) {
			if sel.Sel.Name == "Error" {
				pos, found = sel.Pos(), true
			}
			return false
		}
		if isErrorExpr(pkg, errorIface, expr) {
			pos, found = expr.Pos(), true
			return false
		}
		return true
	})
	return pos, found
}

// isErrorExpr reports whether expr has a type that implements error.
func isErrorExpr(pkg *packages.Package, errorIface *types.Interface, expr ast.Expr) bool {
	typeAndValue, ok := pkg.TypesInfo.Types[expr]
	return ok && typeAndValue.Type != nil && types.Implements(typeAndValue.Type, errorIface)
}

// TestNoRawErrorTextInResponses fails when error text reaches a gin reply.
// Handlers log the detail and reply with text written for the caller.
func TestNoRawErrorTextInResponses(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	module := modulePath(t, root)
	var production []*packages.Package
	for _, pkg := range loadTypedPackages(t, root, false, "./...") {
		if pkg.PkgPath != module+"/tests" {
			production = append(production, pkg)
		}
	}
	scan := newErrorTextScan(root, production)
	scan.scanReplies()
	if violations := scan.sortedViolations(); len(violations) > 0 {
		t.Errorf("found %d place(s) where error text reaches a response body. Log the error and "+
			"reply with text written for the caller:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestErrorTextScanDetects fails when the scan misses a reply line marked
// "// leak" in the fixture, or reports any other line.
func TestErrorTextScanDetects(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	scan := newErrorTextScan(root, loadTypedPackages(t, root, false, "./"+errorTextFixtureDir))
	scan.scanReplies()

	fixture := errorTextFixtureDir + "/fixture.go"
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasSuffix(line, "// leak") {
			want = append(want, fmt.Sprintf("%s:%d", fixture, i+1))
		}
	}
	got := map[string]bool{}
	for violation := range scan.violations {
		_, sink, _ := strings.Cut(violation, "reaching ")
		got[sink] = true
	}
	if len(want) == 0 || len(got) != len(want) {
		t.Fatalf("reported sinks %v, want exactly %v", got, want)
	}
	for _, sink := range want {
		if !got[sink] {
			t.Errorf("leak at %s was not reported (got %v)", sink, got)
		}
	}
}
