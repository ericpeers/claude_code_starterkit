// SPDX-License-Identifier: MIT

package tests

// Ephemeral test-database lifecycle with a production-data safety guard. The
// rules for when the database is required are in CLAUDE.md under Testing.
// Customize testDBName, productionRowThreshold, guardTable, and the seed calls.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// schemaFile is the DDL file, repo-relative. Its presence makes the database required unless SKIP_DB=1.
const schemaFile = "create_tables.sql"

const (
	testDBName             = "app_test"
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

	// Load .env from the repo root so `go test ./tests/` picks up PG_URL with
	// nothing exported by hand. A shell variable already set takes precedence.
	_ = godotenv.Load("../.env", ".env")

	// SKIP_DB=1 skips DB setup. So does a fresh scaffold with no schema yet.
	// A schema without PG_URL fails, so a dropped PG_URL does not pass in silence.
	// Past this switch the database must be reachable.
	pgURL := os.Getenv("PG_URL")
	_, schemaErr := os.Stat("../" + schemaFile)
	schemaExists := schemaErr == nil

	switch {
	case os.Getenv("SKIP_DB") == "1":
		fmt.Fprintln(os.Stderr, "SKIP_DB=1 — skipping DB setup; DB-dependent tests will skip.")
		return m.Run()
	case !schemaExists:
		fmt.Fprintf(os.Stderr, "%s not present yet — no DB-backed tests to run; skipping DB setup.\n", schemaFile)
		return m.Run()
	case pgURL == "":
		fmt.Fprintf(os.Stderr,
			"%s exists but PG_URL is not set. DB-backed tests are required once the project has a schema.\n"+
				"Set PG_URL (setup_dev.sh writes it to .env), or pass SKIP_DB=1 to run only the file-based gates.\n",
			schemaFile)
		return 1
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
