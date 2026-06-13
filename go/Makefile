.PHONY: all test build setup_githooks fix_vulns

all: test build

setup_githooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit

test:
	go test ./tests/ -timeout 120s

# Run the full suite including the DB-backed itest-tagged tests.
test_itest:
	go test -tags itest ./tests/ -timeout 120s

# Update vulnerable dependencies flagged by govulncheck.
fix_vulns:
	govulncheck ./... || true
	go get -u ./...
	go mod tidy

# Version string: YYYY.MM.DD.<commits-today>-<short-hash>
DATE    := $(shell date +%Y.%m.%d)
HASH    := $(shell git rev-parse --short HEAD 2>/dev/null || echo nogit)
COUNT   := $(shell git rev-list --count --since="$$(date +%Y-%m-%d) 00:00:00" HEAD 2>/dev/null || echo 0)
VERSION := $(DATE).$(COUNT)-$(HASH)

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o app .
