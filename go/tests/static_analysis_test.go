// SPDX-License-Identifier: MIT

package tests

// Static-analysis and vulnerability gates. The third-party tools run through
// `go tool`, at the versions in go.mod's tool block, so no global install is needed.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// runToolGate runs one command in the module root and fails with its output on
// a non-zero exit.
func runToolGate(t *testing.T, label string, cmd *exec.Cmd) {
	t.Helper()
	cmd.Dir = getRepoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("%s found issues (%v):\n%s", label, err, string(output))
	}
}

// TestStaticcheck runs staticcheck over the module. staticcheck.conf at the
// root holds the suppressions.
func TestStaticcheck(t *testing.T) {
	t.Parallel()
	runToolGate(t, "staticcheck", goTool("staticcheck", "./..."))
}

// TestUnparam finds unused parameters and results. It passes -exported, since
// an application module has no outside callers to break.
func TestUnparam(t *testing.T) {
	t.Parallel()
	runToolGate(t, "unparam", goTool("unparam", "-exported", "./..."))
}

// TestGoVet runs go vet over the module.
func TestGoVet(t *testing.T) {
	t.Parallel()
	runToolGate(t, "go vet", exec.Command("go", "vet", "./..."))
}

// TestIneffassign flags assignments whose value is overwritten before any read.
// The usual case is a shadowed err that drops an error.
func TestIneffassign(t *testing.T) {
	t.Parallel()
	runToolGate(t, "ineffassign", goTool("ineffassign", "./..."))
}

// TestGosec runs gosec over the module. Suppress a finding with #nosec at the
// call site, with the reason.
func TestGosec(t *testing.T) {
	t.Parallel()
	runToolGate(t, "gosec", goTool("gosec", "-quiet", "./..."))
}

// TestGofmt fails if any Go file is not gofmt-formatted.
func TestGofmt(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)

	cmd := exec.Command("gofmt", "-l", ".")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gofmt failed to run: %v\n%s", err, string(output))
	}
	if files := strings.TrimSpace(string(output)); files != "" {
		t.Errorf("these files are not gofmt-formatted (run `gofmt -w .`):\n%s", files)
	}
}

// TestGoModTidy fails if go.mod or go.sum is not tidy. The -diff flag reports
// the change without writing it.
func TestGoModTidy(t *testing.T) {
	t.Parallel()
	runToolGate(t, "go mod tidy -diff (run `go mod tidy`)", exec.Command("go", "mod", "tidy", "-diff"))
}

// deadcodeAllowlist holds functions kept although production code does not call
// them, each with a reason. Keys look like "internal/foo/bar.go:Func" or
// "internal/foo/bar.go:Type.Method".
var deadcodeAllowlist = map[string]bool{}

// deadcodeMarker separates the location from the name in a deadcode line.
const deadcodeMarker = ": unreachable func: "

// TestDeadcode fails on a function no main package reaches. It skips until the
// module has a main package, since deadcode starts from main.
func TestDeadcode(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	mains := mainPackages(t, root)
	if len(mains) == 0 {
		t.Skip("no main package yet; deadcode starts from main")
	}

	cmd := goTool("deadcode", mains...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deadcode failed to run: %v\n%s", err, string(output))
	}

	var violations []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		key, found := deadcodeKey(line)
		if found && !deadcodeAllowlist[key] {
			violations = append(violations, fmt.Sprintf("  %s (key: %q)", line, key))
		}
	}

	if len(violations) > 0 {
		t.Errorf("deadcode found %d unreachable function(s). Delete them, or add the key to "+
			"deadcodeAllowlist with a reason:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// deadcodeKey turns one deadcode line into its allowlist key, "path.go:Name". It
// reports false for a line that names no unreachable function.
func deadcodeKey(line string) (string, bool) {
	location, name, found := strings.Cut(strings.TrimSpace(line), deadcodeMarker)
	if !found {
		return "", false
	}
	// Strip ":line:col" from the end, so a drive letter such as C: survives.
	file := location
	for range 2 {
		if colon := strings.LastIndex(file, ":"); colon != -1 {
			file = file[:colon]
		}
	}
	return file + ":" + name, true
}

// TestDeadcodeKey fails when a deadcode line maps to the wrong allowlist key.
func TestDeadcodeKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line    string
		wantKey string
		wantOK  bool
	}{
		{"internal/orders/repo.go:12:6: unreachable func: Repo.Purge", "internal/orders/repo.go:Repo.Purge", true},
		{`  C:\app\internal\orders.go:3:1: unreachable func: helper`, `C:\app\internal\orders.go:helper`, true},
		{"go: downloading example.com/mod v1.0.0", "", false},
	}
	for _, tc := range cases {
		if key, found := deadcodeKey(tc.line); key != tc.wantKey || found != tc.wantOK {
			t.Errorf("deadcodeKey(%q) = %q, %v; want %q, %v", tc.line, key, found, tc.wantKey, tc.wantOK)
		}
	}
}

// TestInternalPackagesReachable fails on an internal package no main package
// imports. deadcode loads only imported packages, so it is blind to such a one.
func TestInternalPackagesReachable(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	mains := mainPackages(t, root)
	if len(mains) == 0 {
		t.Skip("no main package yet; nothing imports internal/")
	}

	reachable := map[string]bool{}
	for _, pkg := range goList(t, root, append([]string{"-deps"}, mains...)...) {
		reachable[pkg] = true
	}
	var orphans []string
	for _, pkg := range goList(t, root, "./internal/...") {
		if !reachable[pkg] {
			orphans = append(orphans, "  "+pkg)
		}
	}
	if len(orphans) > 0 {
		t.Errorf("%d internal package(s) are not imported by a main package, so only tests reach them.\n"+
			"Wire them into main or delete them:\n%s", len(orphans), strings.Join(orphans, "\n"))
	}
}

// unnamedInterfaceMethods are methods the standard library calls through
// interfaces it does not name.
var unnamedInterfaceMethods = map[string]bool{
	"Unwrap": true, // errors.Unwrap, errors.Is and errors.As
	"Is":     true, // errors.Is
	"As":     true, // errors.As
	"IsZero": true, // encoding/json omitzero
}

// TestNoUncalledExportedMethods fails on an exported method nothing uses.
// deadcode counts every method of a type stored in an interface as reachable,
// so this gate looks for a use of each one. Packages under optional/ are left out.
func TestNoUncalledExportedMethods(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	module := modulePath(t, root)

	pkgs := loadTypedPackages(t, root, false, "./...")

	used := map[*types.Func]bool{}
	var methods []*types.Func
	for _, pkg := range pkgs {
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok {
				used[fn.Origin()] = true
			}
		}
		if strings.HasPrefix(pkg.PkgPath, module+"/"+optionalDir+"/") {
			continue
		}
		for _, obj := range pkg.TypesInfo.Defs {
			fn, ok := obj.(*types.Func)
			if !ok || !fn.Exported() {
				continue
			}
			if recv := fn.Signature().Recv(); recv != nil && !types.IsInterface(recv.Type()) {
				methods = append(methods, fn)
			}
		}
	}

	// A method that satisfies an interface may be called only through it.
	interfacesByMethod := namedInterfacesByMethod(pkgs)
	var violations []string
	for _, fn := range methods {
		if used[fn] || unnamedInterfaceMethods[fn.Name()] || satisfiesInterfaceMethod(fn, interfacesByMethod[fn.Name()]) {
			continue
		}
		pos := pkgs[0].Fset.Position(fn.Pos())
		rel := filepath.ToSlash(relFromRoot(root, pos.Filename))
		key := fmt.Sprintf("%s:%s.%s", rel, receiverTypeName(fn), fn.Name())
		if !deadcodeAllowlist[key] {
			violations = append(violations, fmt.Sprintf("  %s:%d (key: %q)", rel, pos.Line, key))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("found %d exported method(s) that no code uses. Delete them, or add the key to "+
			"deadcodeAllowlist with a reason:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// namedInterfacesByMethod maps each method name to the named interfaces that
// declare it, across the loaded packages and their imports.
func namedInterfacesByMethod(pkgs []*packages.Package) map[string][]*types.Interface {
	errorIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	byMethod := map[string][]*types.Interface{"Error": {errorIface}}
	seen := map[*types.Package]bool{}
	for _, pkg := range pkgs {
		collectInterfaces(pkg.Types, seen, byMethod)
	}
	return byMethod
}

// collectInterfaces adds the named interfaces of pkg and its imports to byMethod.
func collectInterfaces(pkg *types.Package, seen map[*types.Package]bool, byMethod map[string][]*types.Interface) {
	if seen[pkg] {
		return
	}
	seen[pkg] = true
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		typeName, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := typeName.Type().(*types.Named)
		if !ok || named.TypeParams().Len() > 0 {
			continue
		}
		iface, ok := named.Underlying().(*types.Interface)
		if !ok {
			continue
		}
		for i := range iface.NumMethods() {
			byMethod[iface.Method(i).Name()] = append(byMethod[iface.Method(i).Name()], iface)
		}
	}
	for _, imported := range pkg.Imports() {
		collectInterfaces(imported, seen, byMethod)
	}
}

// satisfiesInterfaceMethod reports whether the method's receiver type
// implements one of ifaces.
func satisfiesInterfaceMethod(fn *types.Func, ifaces []*types.Interface) bool {
	recv := fn.Signature().Recv().Type()
	if _, isPtr := recv.(*types.Pointer); !isPtr {
		recv = types.NewPointer(recv)
	}
	for _, iface := range ifaces {
		if types.Implements(recv, iface) {
			return true
		}
	}
	return false
}

// receiverTypeName returns the name of the method's receiver type, without a pointer.
func receiverTypeName(fn *types.Func) string {
	recv := fn.Signature().Recv().Type()
	if ptr, ok := recv.(*types.Pointer); ok {
		recv = ptr.Elem()
	}
	if named, ok := types.Unalias(recv).(*types.Named); ok {
		return named.Obj().Name()
	}
	return recv.String()
}

// vulnAllowlistPath holds govulncheck findings that have no fix yet, each with a
// reason. scripts/fix_vulns.sh appends to it.
var vulnAllowlistPath = filepath.Join("tests", "vuln_allowlist.json")

// vulnFinding is one govulncheck finding, as its JSON stream reports it.
type vulnFinding struct {
	OSV          string `json:"osv"`
	FixedVersion string `json:"fixed_version"`
	Trace        []struct {
		Function string `json:"function"`
	} `json:"trace"`
}

// vulnMessage is one message of the govulncheck JSON stream. Only findings and
// OSV records matter here.
type vulnMessage struct {
	Finding *vulnFinding `json:"finding"`
	OSV     *struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"osv"`
}

// vulnState is what the stream says about one OSV id. It is reachable when a
// finding's trace names a function, and fixable when a finding names a fixed
// version. scripts/fix_vulns.sh applies the same rule.
type vulnState struct {
	reachable bool
	fixed     string
	summary   string
}

// decodeVulnStream folds the govulncheck JSON stream into one state per OSV id.
func decodeVulnStream(t *testing.T, out []byte) map[string]*vulnState {
	t.Helper()
	state := map[string]*vulnState{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var msg vulnMessage
		if err := dec.Decode(&msg); err != nil {
			t.Fatalf("decoding govulncheck JSON: %v", err)
		}
		if msg.OSV != nil {
			vulnEntry(state, msg.OSV.ID).summary = msg.OSV.Summary
		}
		if finding := msg.Finding; finding != nil {
			entry := vulnEntry(state, finding.OSV)
			if finding.FixedVersion != "" {
				entry.fixed = finding.FixedVersion
			}
			if len(finding.Trace) > 0 && finding.Trace[0].Function != "" {
				entry.reachable = true
			}
		}
	}
	return state
}

// vulnEntry returns the state for id, adding an empty one on first sight.
func vulnEntry(state map[string]*vulnState, id string) *vulnState {
	if state[id] == nil {
		state[id] = &vulnState{}
	}
	return state[id]
}

// readVulnAllowlist returns the allowlisted OSV ids. A missing file reads as empty.
func readVulnAllowlist(t *testing.T, root string) map[string]bool {
	t.Helper()
	var file struct {
		Allowlist []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"allowlist"`
	}
	data, err := os.ReadFile(filepath.Join(root, vulnAllowlistPath))
	if os.IsNotExist(err) {
		return map[string]bool{}
	}
	if err != nil {
		t.Fatalf("reading %s: %v", vulnAllowlistPath, err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parsing %s: %v", vulnAllowlistPath, err)
	}
	allowed := map[string]bool{}
	for _, entry := range file.Allowlist {
		allowed[entry.ID] = true
	}
	return allowed
}

// vulnTriage sorts the reachable findings by the action each one needs. An
// allowlisted finding with no fix needs none.
type vulnTriage struct {
	needUpgrade         []string
	needTriage          []string
	allowlistedButFixed []string
}

// classifyVulns sorts each reachable OSV id into a triage list, one report line each.
func classifyVulns(state map[string]*vulnState, allowed map[string]bool) vulnTriage {
	var triage vulnTriage
	for id, entry := range state {
		if !entry.reachable {
			continue
		}
		switch {
		case entry.fixed != "" && allowed[id]:
			triage.allowlistedButFixed = append(triage.allowlistedButFixed,
				fmt.Sprintf("  %s (now fixed in %s): %s", id, entry.fixed, entry.summary))
		case entry.fixed != "":
			triage.needUpgrade = append(triage.needUpgrade, fmt.Sprintf("  %s (fixed in %s): %s", id, entry.fixed, entry.summary))
		case !allowed[id]:
			triage.needTriage = append(triage.needTriage, fmt.Sprintf("  %s: %s", id, entry.summary))
		}
	}
	return triage
}

// vulnStreamFixture is a canned govulncheck stream with one finding per triage
// outcome, plus an unreachable one and an allowlisted one with no fix.
const vulnStreamFixture = `
{"osv": {"id": "GO-1", "summary": "fixable"}}
{"finding": {"osv": "GO-1", "fixed_version": "v1.2.0", "trace": [{"function": "Parse"}]}}
{"osv": {"id": "GO-2", "summary": "no fix yet"}}
{"finding": {"osv": "GO-2", "trace": [{"function": "Dial"}]}}
{"osv": {"id": "GO-3", "summary": "allowlisted, now fixed"}}
{"finding": {"osv": "GO-3", "fixed_version": "v2.0.1", "trace": [{"function": "Read"}]}}
{"osv": {"id": "GO-4", "summary": "imported but not called"}}
{"finding": {"osv": "GO-4", "fixed_version": "v0.3.0", "trace": [{"module": "example.com/lib"}]}}
{"osv": {"id": "GO-5", "summary": "allowlisted, no fix"}}
{"finding": {"osv": "GO-5", "trace": [{"function": "Write"}]}}
`

// TestClassifyVulns fails when a finding lands in the wrong triage list.
func TestClassifyVulns(t *testing.T) {
	t.Parallel()
	triage := classifyVulns(decodeVulnStream(t, []byte(vulnStreamFixture)), map[string]bool{"GO-3": true, "GO-5": true})
	checks := []struct {
		name string
		got  []string
		want []string
	}{
		{"needUpgrade", triage.needUpgrade, []string{"  GO-1 (fixed in v1.2.0): fixable"}},
		{"needTriage", triage.needTriage, []string{"  GO-2: no fix yet"}},
		{"allowlistedButFixed", triage.allowlistedButFixed, []string{"  GO-3 (now fixed in v2.0.1): allowlisted, now fixed"}},
	}
	for _, check := range checks {
		if fmt.Sprint(check.got) != fmt.Sprint(check.want) {
			t.Errorf("%s = %q, want %q", check.name, check.got, check.want)
		}
	}
}

// TestGovulncheck checks dependencies and the standard library against the Go
// vulnerability database. It fails on a reachable finding with a fix, or one
// missing from the allowlist, and on an allowlisted one that now has a fix.
func TestGovulncheck(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)

	cmd := goTool("govulncheck", "-format", "json", "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("govulncheck failed: %v\n%s", err, stderr.String())
	}

	triage := classifyVulns(decodeVulnStream(t, out), readVulnAllowlist(t, root))
	var sections []string
	for _, section := range []struct {
		heading string
		lines   []string
	}{
		{"Vulnerable dependencies with fixes available. Run 'make fix_vulns'", triage.needUpgrade},
		{"Allowlisted vulnerabilities now have fixes. Run 'make fix_vulns' to apply them", triage.allowlistedButFixed},
		{"Vulnerabilities with no fix, missing from the allowlist. Run 'make fix_vulns' to triage", triage.needTriage},
	} {
		if len(section.lines) > 0 {
			sort.Strings(section.lines)
			sections = append(sections, section.heading+":\n"+strings.Join(section.lines, "\n"))
		}
	}
	if len(sections) > 0 {
		t.Errorf("govulncheck audit failed:\n\n%s", strings.Join(sections, "\n\n"))
	}
}
