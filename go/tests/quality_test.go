// SPDX-License-Identifier: MIT

package tests

// Generic, file-based quality gates. These need no database and run as soon as
// the project has an `internal/` tree and a SQL schema file. Each test skips
// gracefully when its target paths don't exist yet, so a fresh scaffold stays
// green until you add the code these guard.
//
// Customize:
//   - schemaFile        : path (repo-relative) to your DDL file
//   - repositoryDir     : path to the package whose table access you want fenced
//   - tableOwnership    : map every table to the single file allowed to mutate it
//   - allowedJoins      : read-only cross-file JOIN exceptions

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	schemaFile    = "create_tables.sql"
	repositoryDir = "internal/repository"
)

// TestSQLFluffLint runs `sqlfluff lint` on the schema file. Skips if sqlfluff is
// not installed or the schema file does not exist.
func TestSQLFluffLint(t *testing.T) {
	t.Parallel()
	schemaPath := optionalFilePath(t, schemaFile)
	if schemaPath == "" {
		t.Skipf("%s not present yet; nothing to lint", schemaFile)
	}
	if _, err := exec.LookPath("sqlfluff"); err != nil {
		t.Skip("sqlfluff not installed; skipping SQL lint")
	}

	cmd := exec.Command("sqlfluff", "lint", schemaPath, "--dialect", "postgres")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if !strings.Contains(outputStr, "All Finished") {
		t.Errorf("sqlfluff did not complete. Expected 'All Finished'.\nOutput:\n%s\nErr: %v", outputStr, err)
	}
	if strings.Contains(outputStr, "FAIL") {
		t.Errorf("sqlfluff found errors (try `sqlfluff fix`):\n%s", outputStr)
	}
}

// TestRepositoryTableOwnership enforces that each repository file only accesses
// tables it owns. This makes module-boundary isolation a mechanical gate rather
// than a code-review habit. Read-only cross-file JOINs are whitelisted in
// allowedJoins. Skips if the repository directory does not exist yet.
//
// Fill in tableOwnership/allowedJoins for your schema. The entries below are an
// illustrative example — replace them.
func TestRepositoryTableOwnership(t *testing.T) {
	t.Parallel()

	// EXAMPLE — replace with your own table -> owning-file map.
	tableOwnership := map[string]string{
		"users":    "user_repo.go",
		"orders":   "order_repo.go",
		"products": "product_repo.go",
	}
	// EXAMPLE — read-only JOIN exceptions: table -> files allowed to JOIN it.
	allowedJoins := map[string][]string{
		"users": {"order_repo.go"}, // order_repo JOINs users for display name
	}

	repoPath := optionalFilePath(t, repositoryDir)
	if repoPath == "" {
		t.Skipf("%s not present yet; nothing to check", repositoryDir)
	}

	entries, err := os.ReadDir(repoPath)
	if err != nil {
		t.Fatalf("read repository dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_repo.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(repoPath, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}

		for _, table := range extractTablesFromQueries(string(content)) {
			owner, known := tableOwnership[table]
			if !known {
				t.Errorf("table %q accessed in %s is not in tableOwnership — add it and assign an owner",
					table, entry.Name())
				continue
			}
			if owner == entry.Name() {
				continue
			}
			if allowed, ok := allowedJoins[table]; ok && contains(allowed, entry.Name()) {
				continue
			}
			t.Errorf("table ownership violation: table %q accessed in %s but owned by %s",
				table, entry.Name(), owner)
		}
	}
}

// TestNoImplicitJoins ensures all SQL uses explicit JOIN ... ON syntax rather
// than the old `FROM a, b WHERE a.id = b.id` comma form. Skips if internal/
// does not exist yet.
func TestNoImplicitJoins(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	internalDir := filepath.Join(root, "internal")
	if _, err := os.Stat(internalDir); err != nil {
		t.Skip("internal/ not present yet")
	}

	implicitJoin := regexp.MustCompile(`(?i)\bFROM\s+\w+\s*,`)
	backtick := regexp.MustCompile("`[^`]+`")

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(content)
		for _, loc := range backtick.FindAllStringIndex(s, -1) {
			if implicitJoin.MatchString(s[loc[0]:loc[1]]) {
				line := strings.Count(s[:loc[0]], "\n") + 1
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d: implicit JOIN detected. Use explicit JOIN ... ON syntax.", rel, line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}

// --- helpers ---

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

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
// when it does not exist (so callers can t.Skip instead of failing).
func optionalFilePath(t *testing.T, relativePath string) string {
	t.Helper()
	full := filepath.Join(getRepoRoot(t), relativePath)
	if _, err := os.Stat(full); err != nil {
		return ""
	}
	return full
}

// extractTablesFromQueries pulls table names out of backtick-delimited SQL
// strings in Go source, ignoring SQL keywords. Only backtick strings are parsed
// to avoid false positives from comments and error messages.
func extractTablesFromQueries(content string) []string {
	tables := make(map[string]bool)
	keywords := map[string]bool{
		"set": true, "where": true, "and": true, "or": true,
		"select": true, "from": true, "join": true, "on": true,
		"insert": true, "into": true, "update": true, "delete": true,
		"values": true, "null": true, "not": true, "in": true,
		"order": true, "by": true, "asc": true, "desc": true,
		"limit": true, "offset": true, "group": true, "having": true,
		"left": true, "right": true, "inner": true, "outer": true,
		"cross": true, "full": true, "lateral": true,
		"unnest": true, "generate_series": true,
		"excluded": true, "conflict": true, "do": true,
	}

	backtick := regexp.MustCompile("`[^`]+`")
	from := regexp.MustCompile(`(?i)\bFROM\s+(\w+)`)
	join := regexp.MustCompile(`(?i)\bJOIN\s+(\w+)`)
	insert := regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(\w+)`)
	update := regexp.MustCompile(`(?i)\bUPDATE\s+(\w+)`)
	del := regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+(\w+)`)

	add := func(name string) {
		if lower := strings.ToLower(name); !keywords[lower] {
			tables[lower] = true
		}
	}
	for _, sqlStr := range backtick.FindAllString(content, -1) {
		for _, re := range []*regexp.Regexp{from, join, insert, update, del} {
			for _, m := range re.FindAllStringSubmatch(sqlStr, -1) {
				add(m[1])
			}
		}
	}

	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}
	return result
}
