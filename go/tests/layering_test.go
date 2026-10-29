// SPDX-License-Identifier: MIT

package tests

// Layering gates for the handlers → services → repository flow in CLAUDE.md.

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// pgxGuardedDirs are the layers above the repository. They reach the database
// only through repository methods.
var pgxGuardedDirs = []string{"internal/services", "internal/handlers", "internal/models"}

// pgxImportPrefix covers pgx and its subpackages: pgxpool, pgtype, pgconn.
const pgxImportPrefix = "github.com/jackc/pgx"

// pgxImportViolations reports each non-test file under dirs that imports pgx.
// scanned counts the files read per directory, and a missing directory is absent.
func pgxImportViolations(root string, dirs []string) (violations []string, scanned map[string]int, err error) {
	scanned = map[string]int{}
	for _, dir := range dirs {
		base := filepath.Join(root, filepath.FromSlash(dir))
		if _, statErr := os.Stat(base); statErr != nil {
			continue
		}
		scanned[dir] = 0
		// The callback adds to violations and scanned, which WalkDir has no way to return.
		walkErr := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return err
			}
			scanned[dir]++
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return fmt.Errorf("parsing %s: %w", path, parseErr)
			}
			for _, imp := range parsed.Imports {
				importPath, unquoteErr := strconv.Unquote(imp.Path.Value)
				if unquoteErr == nil && strings.HasPrefix(importPath, pgxImportPrefix) {
					violations = append(violations, fmt.Sprintf("%s imports %q", filepath.ToSlash(relFromRoot(root, path)), importPath))
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, nil, walkErr
		}
	}
	return violations, scanned, nil
}

// TestNoPgxImportsAboveRepository keeps the Postgres driver out of service,
// handler, and model code. It skips until one of those packages exists.
func TestNoPgxImportsAboveRepository(t *testing.T) {
	t.Parallel()
	violations, scanned, err := pgxImportViolations(getRepoRoot(t), pgxGuardedDirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) == 0 {
		t.Skipf("none of %v exist yet", pgxGuardedDirs)
	}
	for dir, count := range scanned {
		// An empty package would let this gate pass while guarding nothing.
		if count == 0 {
			t.Errorf("%s has no non-test .go files. Was it moved? Update pgxGuardedDirs.", dir)
		}
	}
	if len(violations) > 0 {
		t.Errorf("pgx belongs to the repository layer. Route the need through a repository method "+
			"that returns a domain type:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestPgxImportViolationsDetects runs the import scan over a temporary tree.
func TestPgxImportViolationsDetects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := map[string]string{
		"internal/services/order_service.go": "package services\n\nimport _ \"github.com/jackc/pgx/v5/pgxpool\"\n",
		"internal/services/order_rules.go":   "package services\n\nimport _ \"strings\"\n",
		"internal/repository/order_repo.go":  "package repository\n\nimport _ \"github.com/jackc/pgx/v5\"\n",
	}
	for rel, src := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "handlers"), 0o755); err != nil {
		t.Fatal(err)
	}

	violations, scanned, err := pgxImportViolations(root, pgxGuardedDirs)
	if err != nil {
		t.Fatal(err)
	}
	want := `internal/services/order_service.go imports "github.com/jackc/pgx/v5/pgxpool"`
	if len(violations) != 1 || violations[0] != want {
		t.Errorf("violations = %q, want [%q]", violations, want)
	}
	if fmt.Sprint(scanned) != "map[internal/handlers:0 internal/services:2]" {
		t.Errorf("scanned = %v, want two service files, an empty handlers dir, and no models entry", scanned)
	}
}
