// SPDX-License-Identifier: MIT

package tests

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixVulnsStream holds two fixable findings in one module and one unreachable.
// GO-0004 has no fix, so govulncheck omits its fixed_version.
const fixVulnsStream = `
{"osv": {"id": "GO-0001", "summary": "first"}}
{"finding": {"osv": "GO-0001", "fixed_version": "v1.2.0", "trace": [{"module": "example.com/a", "function": "Parse"}]}}
{"finding": {"osv": "GO-0002", "fixed_version": "v1.3.0", "trace": [{"module": "example.com/a", "function": "Load"}]}}
{"finding": {"osv": "GO-0003", "fixed_version": "v2.0.0", "trace": [{"module": "example.com/b"}]}}
{"finding": {"osv": "GO-0004", "trace": [{"module": "example.com/c", "function": "Run"}]}}
`

// TestFixVulnsJqFilters fails when fix_vulns.sh would upgrade an unreachable or
// unfixable finding, or would triage an unfixable one as still fixable.
func TestFixVulnsJqFilters(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}
	scriptsDir := filepath.Join(getRepoRoot(t), "scripts")
	cases := []struct {
		filter string
		want   []string
	}{
		{"vuln_upgrades.jq", []string{"example.com/a\tv1.2.0", "example.com/a\tv1.3.0"}},
		{"vuln_remaining.jq", []string{"GO-0001\tv1.2.0", "GO-0002\tv1.3.0", "GO-0004\t"}},
	}
	for _, tc := range cases {
		got := runJqFilter(t, filepath.Join(scriptsDir, tc.filter), fixVulnsStream)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s:\n got %q\nwant %q", tc.filter, got, tc.want)
		}
	}
}

// runJqFilter runs a jq program file over input and returns its output lines.
func runJqFilter(t *testing.T, programPath, input string) []string {
	t.Helper()
	cmd := exec.Command("jq", "-r", "-f", programPath)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jq -f %s: %v", programPath, err)
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}
