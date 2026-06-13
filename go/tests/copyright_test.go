// SPDX-License-Identifier: MIT

package tests

// License / copyright header gate for Go sources.
//
// Two kinds of files coexist in a project built from this kit:
//   * Kit-supplied boilerplate — declares `SPDX-License-Identifier: MIT` and is
//     EXEMPT here. It carries no personal copyright holder by design.
//   * Your own source — must carry a current-year copyright header naming the
//     project's configured holder (see the .copyright-holder file, written by
//     setup_dev.sh). This is how you enforce your ownership on the code you write.
//
// A fresh scaffold is green (everything shipped is SPDX-exempt). The gate starts
// enforcing once you add your own .go files without an SPDX tag or copyright header.

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var spdxRe = regexp.MustCompile(`SPDX-License-Identifier:\s*\S+`)

// expectedHolder returns the configured copyright holder from the committed
// .copyright-holder file (preferred) or the COPYRIGHT_HOLDER env var, or "".
func expectedHolder(t *testing.T) string {
	t.Helper()
	root := getRepoRoot(t)
	if b, err := os.ReadFile(filepath.Join(root, ".copyright-holder")); err == nil {
		if h := strings.TrimSpace(string(b)); h != "" {
			return h
		}
	}
	return strings.TrimSpace(os.Getenv("COPYRIGHT_HOLDER"))
}

func firstLines(content string, n int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestLicenseHeaders(t *testing.T) {
	root := getRepoRoot(t)
	currentYear := time.Now().Year()
	currentYearStr := strconv.Itoa(currentYear)

	holder := expectedHolder(t)
	var presenceRe *regexp.Regexp
	if holder != "" {
		// Presence only: any year or year-range followed by the holder. The year's
		// currency is checked separately against the file's last git-commit year,
		// so a dormant prior-year file isn't forced to bump on every Jan 1.
		presenceRe = regexp.MustCompile(`Copyright \(c\) \d{4}(?:-\d{4})?\s+` + regexp.QuoteMeta(holder))
	}

	var holderUnset []string // holder not set, but non-SPDX files exist
	var needHeader []string  // holder set, but file lacks SPDX and a holder header
	var needYear []string    // header present but missing the required current year

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", ".venv":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		head := firstLines(string(b), 5)
		if spdxRe.MatchString(head) {
			return nil // SPDX-tagged files are exempt
		}
		rel, _ := filepath.Rel(root, path)
		if presenceRe == nil {
			holderUnset = append(holderUnset, rel)
			return nil
		}
		if !presenceRe.MatchString(head) {
			needHeader = append(needHeader, rel)
			return nil
		}
		// Year-currency: require the current year only when the file was last
		// committed this year, or when its commit year is unknown — uncommitted,
		// brand-new, or git unavailable (gitLastCommitYear returns 0). New files
		// must be current; a dormant prior-year file keeps its existing header.
		lastYear := gitLastCommitYear(root, rel)
		if (lastYear == 0 || lastYear == currentYear) && !strings.Contains(head, currentYearStr) {
			needYear = append(needYear, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}

	if presenceRe == nil {
		if len(holderUnset) > 0 {
			t.Fatalf("copyright holder not set. Create .copyright-holder (or run ./setup_dev.sh), "+
				"then add\n  // Copyright (c) %d <holder>\nto these files (or give them an SPDX identifier):\n%s",
				currentYear, strings.Join(holderUnset, "\n"))
		}
		return
	}
	if len(needHeader) > 0 {
		t.Errorf("files missing an SPDX tag or a `Copyright (c) <year> %s` header:\n%s",
			holder, strings.Join(needHeader, "\n"))
	}
	if len(needYear) > 0 {
		t.Errorf("copyright header must include %d (file changed this year):\n%s",
			currentYear, strings.Join(needYear, "\n"))
	}
}

// gitLastCommitYear returns the year of the most recent commit that touched
// relPath (relative to root), or 0 if git is unavailable or the file has never
// been committed. Callers treat 0 as the current year, so new/uncommitted files
// must carry the current year.
func gitLastCommitYear(root, relPath string) int {
	cmd := exec.Command("git", "log", "--follow", "-1", "--format=%ad", "--date=format:%Y", "--", relPath)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	year, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return year
}
