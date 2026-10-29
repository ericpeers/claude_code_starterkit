// SPDX-License-Identifier: MIT

package tests

// Locating the repository and its files, for every gate in the suite.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// getRepoRoot walks up from the working directory to the nearest .git or go.mod.
func getRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

// optionalFilePath returns the absolute path for a repo-relative path, or ""
// when it does not exist.
func optionalFilePath(t *testing.T, relativePath string) string {
	t.Helper()
	full := filepath.Join(getRepoRoot(t), relativePath)
	if _, err := os.Stat(full); err != nil {
		return ""
	}
	return full
}

// requireGitWorkTree skips the test when root is not inside a git work tree. It
// lists one path, so the check stays cheap.
func requireGitWorkTree(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--", ".gitignore")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git ls-files is unavailable (run `git init`): %v\n%s", err, output)
	}
}

// trackedFiles returns the repo-relative files git tracks below root. It skips
// the test when root is not inside a git work tree.
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()
	requireGitWorkTree(t, root)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel != "" {
			files = append(files, rel)
		}
	}
	return files
}
