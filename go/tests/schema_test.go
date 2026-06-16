// SPDX-License-Identifier: MIT

package tests

// Schema-drift detection: verifies the live (test) database schema matches the
// checked-in DDL file column-for-column. Catches an incomplete pg_restore,
// untracked ALTERs, or a binary built against a different schema than was
// applied. Skips when no test database is configured (testPool nil, set up in
// setup_test.go), so it runs as part of the single `go test ./tests/` suite.

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestSchemaMatchesDatabase compares every table/column in the DDL file against
// information_schema. Skips when no test database is configured.
func TestSchemaMatchesDatabase(t *testing.T) {
	t.Parallel()
	if testPool == nil {
		t.Skip("no test database configured (PG_URL unset)")
	}
	schemaPath := optionalFilePath(t, schemaFile)
	if schemaPath == "" {
		t.Skipf("%s not present", schemaFile)
	}

	expected := parseSchemaFields(t, schemaPath)

	rows, err := testPool.Query(t.Context(), `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		t.Fatalf("query information_schema.columns: %v", err)
	}
	defer rows.Close()

	actual := make(map[string]map[string]bool)
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		if actual[tbl] == nil {
			actual[tbl] = make(map[string]bool)
		}
		actual[tbl][col] = true
	}

	// Every table/column in the DDL must exist in the live DB.
	for tbl, cols := range expected {
		if _, ok := actual[tbl]; !ok {
			t.Errorf("table %q is in %s but missing from the live DB", tbl, schemaFile)
			continue
		}
		for col := range cols {
			if !actual[tbl][col] {
				t.Errorf("column %q of table %q is in %s but missing from the live DB", col, tbl, schemaFile)
			}
		}
	}

	// In-flight feature-branch tables that exist in the DB but not yet in the
	// committed DDL. Add entries here to silence, remove once the branch merges.
	futureTables := map[string]bool{}

	// Every table in the DB must be in the DDL (catches untracked schema changes).
	for tbl := range actual {
		if futureTables[tbl] {
			continue
		}
		if _, ok := expected[tbl]; !ok {
			t.Errorf("table %q exists in the live DB but is not in %s", tbl, schemaFile)
		}
	}
}

// parseSchemaFields extracts table -> column definitions from a CREATE TABLE
// DDL file, ignoring comments and constraint lines.
func parseSchemaFields(t *testing.T, schemaPath string) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	content := string(raw)

	// Strip comments.
	content = regexp.MustCompile(`--[^\n]*`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(content, "")

	tables := make(map[string]map[string]bool)
	tableRegex := regexp.MustCompile(`(?is)create\s+table\s+(?:if\s+not\s+exists\s+)?(\w+)\s*\((.*?)\);`)
	for _, match := range tableRegex.FindAllStringSubmatch(content, -1) {
		tableName := strings.ToLower(match[1])
		fields := make(map[string]bool)

		scanner := bufio.NewScanner(strings.NewReader(match[2]))
		for scanner.Scan() {
			line := strings.TrimSuffix(strings.TrimSpace(scanner.Text()), ",")
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "primary key") ||
				strings.HasPrefix(lower, "foreign key") ||
				strings.HasPrefix(lower, "unique") ||
				strings.HasPrefix(lower, "constraint") ||
				strings.HasPrefix(lower, "check") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				col := strings.ToLower(parts[0])
				if !strings.HasPrefix(col, "--") {
					fields[col] = true
				}
			}
		}
		tables[tableName] = fields
	}
	return tables
}
