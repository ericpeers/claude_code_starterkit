// SPDX-License-Identifier: MIT

//go:build itest

package tests

// Ephemeral test-database lifecycle with a production-data safety guard.
//
// Build-tagged `itest`: run with `go test -tags itest ./tests/`. Without the tag
// (plain `go test ./tests/`), only the file-based gates in quality_test.go
// compile and run — no pgx dependency, no Postgres required.
//
// REQUIRES: github.com/jackc/pgx/v5 in go.mod and a PG_URL pointing at a local
// Postgres cluster. If PG_URL is unset, DB setup is skipped and only the
// file-based tests (see quality_test.go) run — so a fresh scaffold without a
// database still passes.
//
// TestMain creates a throwaway database (drops any orphan first), applies your
// schema, asserts it does NOT look like production, then runs the suite and
// drops the database. Tests share the package-level testPool.
//
// Customize:
//   - testDBName                  : name of the ephemeral database
//   - productionRowThreshold      : row count above which we assume real data
//   - guardTable                  : a core table to count rows of in the guard
//   - seed*()                     : add your own synthetic seed functions here

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testDBName             = "app_itest"
	productionRowThreshold = 10_000
	guardTable             = "users" // a table that exists in your schema
)

// testPool is the connection pool for the isolated test database. It is nil when
// PG_URL is unset; DB-dependent tests must skip on nil.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()

	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		// No database configured: run only the file-based tests.
		fmt.Fprintln(os.Stderr, "PG_URL not set — skipping DB setup; DB-dependent tests will skip.")
		return m.Run()
	}

	// Connect to the postgres system DB to manage the test DB lifecycle.
	adminURL, err := replaceDBInURL(pgURL, "postgres")
	if err != nil {
		fmt.Fprintf(os.Stderr, "build admin URL: %v\n", err)
		return 1
	}
	adminConn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to postgres system DB: %v\n", err)
		return 1
	}

	if _, err := adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName); err != nil {
		fmt.Fprintf(os.Stderr, "drop orphan test DB: %v\n", err)
		return 1
	}
	if _, err := adminConn.Exec(ctx, "CREATE DATABASE "+testDBName); err != nil {
		fmt.Fprintf(os.Stderr, "create test DB: %v\n", err)
		return 1
	}
	defer func() {
		if testPool != nil {
			testPool.Close()
		}
		// WITH (FORCE) terminates stray connections (Postgres 13+).
		adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName+" WITH (FORCE)")
		adminConn.Close(ctx)
	}()

	testDBURL, err := replaceDBInURL(pgURL, testDBName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build test DB URL: %v\n", err)
		return 1
	}
	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	testPool, err = pgxpool.New(connCtx, testDBURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to test DB: %v\n", err)
		return 1
	}
	if err := testPool.Ping(connCtx); err != nil {
		fmt.Fprintf(os.Stderr, "ping test DB: %v\n", err)
		return 1
	}

	// Point PG_URL at the isolated DB for any test that re-reads it.
	os.Setenv("PG_URL", testDBURL)

	// Apply schema (create_tables.sql lives one level above tests/).
	schemaSQL, err := os.ReadFile("../" + schemaFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", schemaFile, err)
		return 1
	}
	if err := execMultiStatement(ctx, testPool, string(schemaSQL)); err != nil {
		fmt.Fprintf(os.Stderr, "apply schema: %v\n", err)
		return 1
	}

	// Safety guard: refuse to run against what looks like production data.
	if err := checkNotProductionDB(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	// TODO: call your synthetic seed functions here, e.g. seedReferenceData(ctx, testPool).

	return m.Run()
}

// replaceDBInURL swaps the database-name component of a Postgres connection URL.
func replaceDBInURL(pgURL, dbName string) (string, error) {
	u, err := url.Parse(pgURL)
	if err != nil {
		return "", fmt.Errorf("parse PG_URL: %w", err)
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

// execMultiStatement runs a multi-statement SQL string via the simple query
// protocol, which supports DDL batches.
func execMultiStatement(ctx context.Context, pool *pgxpool.Pool, sql string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()
	return conn.Conn().PgConn().Exec(ctx, sql).Close()
}

// checkNotProductionDB returns an error if guardTable has more than
// productionRowThreshold rows, indicating a misconfigured PG_URL pointing at a
// live database.
func checkNotProductionDB(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+guardTable).Scan(&count); err != nil {
		return fmt.Errorf("safety guard: count %s: %w", guardTable, err)
	}
	if count > productionRowThreshold {
		return fmt.Errorf(
			"SAFETY ABORT: %s has %d rows (> %d). Tests must run against an isolated DB, not a live one",
			guardTable, count, productionRowThreshold)
	}
	return nil
}
