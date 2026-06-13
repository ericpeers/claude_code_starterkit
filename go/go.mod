// Scaffold step: replace the module path with your project's import path,
// then run `go mod tidy`. pgx is only needed for the `itest`-tagged DB tests.
module example.com/app

go 1.25.10

require github.com/jackc/pgx/v5 v5.7.1
