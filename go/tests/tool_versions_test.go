// SPDX-License-Identifier: MIT

package tests

// Freshness gates for the Go toolchain and the tools go.mod declares. Both read
// release data over the network, so they fail offline and can fail on the day
// an upstream release ships.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/version"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/semver"
)

// goReleasesURL lists the supported Go releases as JSON.
const goReleasesURL = "https://go.dev/dl/?mode=json"

// maxToolMinorLag is how many minor releases a tool may trail its latest release.
const maxToolMinorLag = 1

// toolModules maps each module that provides a go.mod tool to one tool package in it.
func toolModules(t *testing.T, root string) map[string]string {
	t.Helper()
	fields := goList(t, root, "-f", "{{.ImportPath}} {{.Module.Path}}", "tool")
	if len(fields)%2 != 0 {
		t.Fatalf("go list tool printed %q, want package and module pairs", fields)
	}
	pkgOf := map[string]string{}
	for i := 0; i < len(fields); i += 2 {
		pkgOf[fields[i+1]] = fields[i]
	}
	return pkgOf
}

// TestGoToolsCurrent fails when a go.mod tool is two or more minor releases
// behind. Patch releases are ignored, and so is a tool with no tagged releases.
func TestGoToolsCurrent(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)

	pkgOf := toolModules(t, root)
	mods := make([]string, 0, len(pkgOf))
	for mod := range pkgOf {
		mods = append(mods, mod)
	}

	out := goOutput(t, root, append([]string{"list", "-m", "-u", "-versions", "-json"}, mods...)...)

	var behind []string
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var mod struct {
			Path, Version string
			Versions      []string
			Update        *struct{ Version string }
		}
		if err := dec.Decode(&mod); err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		if mod.Update == nil {
			continue
		}
		if minorsBehind(mod.Version, mod.Versions) > maxToolMinorLag {
			behind = append(behind, fmt.Sprintf("  %s: go.mod has %s, latest is %s. Run: go get -tool %s@latest",
				mod.Path, mod.Version, mod.Update.Version, pkgOf[mod.Path]))
		}
	}
	sort.Strings(behind)
	if len(behind) > 0 {
		t.Errorf("Go tools more than %d minor release behind. Upgrade one tool per command, then run go mod tidy:\n%s",
			maxToolMinorLag, strings.Join(behind, "\n"))
	}
}

// minorsBehind counts the minor releases in versions newer than current's minor.
// Prereleases are skipped, and a new major release counts once per minor.
func minorsBehind(current string, versions []string) int {
	have := semver.MajorMinor(current)
	newer := map[string]bool{}
	for _, ver := range versions {
		if semver.Prerelease(ver) != "" {
			continue
		}
		if majorMinor := semver.MajorMinor(ver); semver.Compare(majorMinor, have) > 0 {
			newer[majorMinor] = true
		}
	}
	return len(newer)
}

// TestMinorsBehind checks the lag count against a fixed version list.
func TestMinorsBehind(t *testing.T) {
	t.Parallel()
	versions := []string{"v0.50.0", "v0.51.0", "v0.51.1", "v0.52.0", "v0.53.0-rc.1"}
	cases := []struct {
		current string
		want    int
	}{
		{"v0.52.0", 0},
		{"v0.51.1", 1},
		{"v0.51.0", 1},
		{"v0.50.0", 2},
		{"v0.49.3", 3},
	}
	for _, tc := range cases {
		if got := minorsBehind(tc.current, versions); got != tc.want {
			t.Errorf("minorsBehind(%s) = %d, want %d", tc.current, got, tc.want)
		}
	}
	if got := minorsBehind("v1.9.0", []string{"v1.9.0", "v2.0.0", "v2.1.0"}); got != 2 {
		t.Errorf("minorsBehind across a major release = %d, want 2", got)
	}
}

// goModGoVersion returns the go directive from go.mod.
func goModGoVersion(t *testing.T, root string) string {
	t.Helper()
	var mod struct{ Go string }
	if err := json.Unmarshal(goOutput(t, root, "mod", "edit", "-json"), &mod); err != nil {
		t.Fatalf("decoding go.mod: %v", err)
	}
	return mod.Go
}

// newestGoPointRelease fetches the release list and returns the newest stable
// point release, such as go1.27.1.
func newestGoPointRelease(t *testing.T) string {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(goReleasesURL)
	if err != nil {
		t.Fatalf("fetching %s: %v", goReleasesURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetching %s: status %s", goReleasesURL, resp.Status)
	}
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		t.Fatalf("decoding %s: %v", goReleasesURL, err)
	}

	target := ""
	for _, release := range releases {
		if release.Stable && isGoPointRelease(release.Version) && (target == "" || version.Compare(release.Version, target) > 0) {
			target = release.Version
		}
	}
	if target == "" {
		t.Fatalf("%s lists no stable point release", goReleasesURL)
	}
	return target
}

// TestGoVersionCurrent fails when go.mod names an older Go series than the
// newest one. A series counts once its first point release ships.
func TestGoVersionCurrent(t *testing.T) {
	t.Parallel()
	goModVersion := goModGoVersion(t, getRepoRoot(t))
	target := newestGoPointRelease(t)
	if version.Compare(version.Lang("go"+goModVersion), version.Lang(target)) < 0 {
		want := strings.TrimPrefix(target, "go")
		t.Errorf("go.mod is on Go %s, and %s is out. Run: go get go@%s && go mod tidy && ./setup_dev.sh",
			goModVersion, target, want)
	}
}

// isGoPointRelease reports whether a version such as go1.27.1 has a patch above zero.
func isGoPointRelease(goVersion string) bool {
	parts := strings.Split(strings.TrimPrefix(goVersion, "go"), ".")
	return len(parts) == 3 && parts[2] != "0"
}

// TestIsGoPointRelease fails when a .0 release, a bare series, or a release
// candidate counts as a point release.
func TestIsGoPointRelease(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{"go1.27.1": true, "go1.27.0": false, "go1.27": false, "go1.27rc1": false}
	for goVersion, want := range cases {
		if got := isGoPointRelease(goVersion); got != want {
			t.Errorf("isGoPointRelease(%s) = %v, want %v", goVersion, got, want)
		}
	}
}
