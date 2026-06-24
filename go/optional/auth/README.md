# Optional auth module (Go)

A minimal, self-issued **JWT + bcrypt-password** auth pipeline for a Go scaffold
built from this kit. Opt-in: the base Go scaffold ships without it; install it
when your service needs login and protected endpoints.

**What you get**
- `POST /api/v1/auth/login` → signed HS256 JWT (24h).
- `GET /api/v1/auth/me` → the canonical protected-endpoint example.
- Gin middleware: `ValidateUser` (permissive, global), `RequireAuth` (401 gate),
  `RequireAdmin` (403 gate), plus `GetUserID` / `MustGetUserID` / `GetRole`.
- A `users` table + `user_role` enum, with a seeded admin row.
- `bin/login` — mint + per-host-cache a token for scripts and the Swagger UI.
- `bin/set_admin_password.py` / `bin/encrypt_admin_passwd.py` — set the admin
  password locally or generate SQL to set it on a remote server.

**Not included** (deliberately core-only): self-service registration, an
admin-approval workflow, organizations, refresh tokens, or any external OAuth/OIDC
provider. The tokens are self-issued — there is no third-party IdP.

## Install

From your scaffolded project root (the one with `go.mod`):

```sh
../claude_code_starterkit/go/optional/auth/install.sh .
```

`install.sh` copies the Go files into `internal/…` (rewriting the import prefix to
your module path), drops `bin/*` and `scripts/setup_admin.sh` in place, appends the
auth vars to `.env_sample`, installs `create_tables.sql` (or tells you to merge if
you already have one), and runs `go mod tidy`. It is idempotent.

## Two manual wiring edits

### 1. `main.go`

After building the router with `gin.New()` + `gin.Recovery()` +
`middleware.GinLogger()`, wire the service and routes:

```go
userRepo := repository.NewUserRepository(pool)
authSvc  := services.NewAuthService(userRepo, cfg.JWTSecret) // or os.Getenv("JWT_SECRET")
authH    := handlers.NewAuthHandler(authSvc)

router.Use(middleware.ValidateUser([]byte(cfg.JWTSecret))) // permissive, global

v1 := router.Group("/api/v1")
a  := v1.Group("/auth")
a.POST("/login", authH.Login)
a.GET("/me", middleware.RequireAuth(), authH.Me)

// Protect your own routes:
//   v1.GET("/things", middleware.RequireAuth(), thingH.List)
// Admin-only group:
//   admin := v1.Group("/admin")
//   admin.Use(middleware.RequireAuth(), middleware.RequireAdmin())
```

For Swagger "Authorize" support, add this security definition to the package-level
doc comment above `main()` (alongside the other `@…` annotations), then `make docs`:

```go
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

### 2. `setup_dev.sh`

After the schema-apply phase (Phase 3c), add one guarded line — same pattern the
kit uses to source `pg_setup.sh`:

```sh
[ -f scripts/setup_admin.sh ] && source scripts/setup_admin.sh
```

On first run it generates `ADMIN_PASS` in `.env` and sets it on the seeded admin
(`admin@example.com`). Re-runs are idempotent.

## Use it

```sh
./setup_dev.sh                                   # provisions DB, applies schema, sets admin password
go run .                                          # start the server
TOKEN="$(bin/login | sed -n 's/^TOKEN: //p')"     # mint a JWT
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/auth/me
```

## Notes

- **`ADMIN_EMAIL` must match the seeded row.** The schema seeds
  `admin@example.com`; if you change `ADMIN_EMAIL` in `.env`, change the
  `INSERT INTO users …` in `create_tables.sql` to match (or `set_admin_password.py`
  finds no row).
- **`ErrorResponse` collision.** The module ships `internal/models/errors.go`
  (`models.ErrorResponse`). If your app already defines that type, delete the
  module's copy after install.
- **Table ownership gate.** `tests/quality_test.go` already maps
  `"users" → user_repo.go`, so the repository-ownership gate passes as-is.
- **Rotating the admin password later:**
  `python3 bin/set_admin_password.py admin@example.com '<new>' "$PG_URL"`, or on a
  remote box: `bin/encrypt_admin_passwd.py admin@example.com '<new>' | psql "$PG_URL"`.
