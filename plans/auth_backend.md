# Self-Issued JWT Auth Pipeline (Backend)

A portable pattern for adding **email/password login + JWT-protected endpoints**
to an existing service, with no external identity provider. The server is its own
authority: it stores a bcrypt password hash, verifies it on login, and mints a
signed JWT it later verifies itself. It has **three legs**, all built around one
signing secret and one `users` table.

| Leg | Where | What it does |
|---|---|---|
| **Authenticate** | `POST /api/v1/auth/login` → service | Verifies the bcrypt hash, then issues an HS256 JWT (24h) carrying the user id + role |
| **Protect** | Gin middleware: `ValidateUser` (global) + `RequireAuth` / `RequireAdmin` (per-route) | Parses `Authorization: Bearer <jwt>`, puts the user in the request context, and gates routes |
| **Bootstrap** | `users` schema + setup script + `bin/login` | Seeds an admin, sets its password out-of-band, and mints/caches tokens for scripts, CI, and the Swagger UI |

This plan assumes the **target repo already exists** — it has a `main.go`/router,
a Postgres pool, a schema or migration system, and its own module path and
header/license conventions. The steps below integrate into those rather than
scaffolding a new project. A complete, tested reference implementation of every
file lives in `claude_code_starterkit/go/optional/auth/`; copy from it where
useful, but the structure below stands on its own.

## Key design decisions

- **Self-issued, not federated.** Tokens are signed and verified with one shared
  `JWT_SECRET` (HS256). There is no OAuth/OIDC provider, no callback, no JWKS.
  This is the right default for a single backend; swapping to an external IdP
  later only changes the *issue* + *verify-key* steps, not the route gating.
- **Fail-closed identity.** Required-auth handlers read the user via a
  `MustGetUserID` that **panics** (→ 500 via recovery) if no user is present,
  rather than falling back to user 0. The panic is only reachable as a wiring
  bug (a protected route missing `RequireAuth`), never as a silent auth bypass.
- **Permissive validate, explicit require.** A single global `ValidateUser`
  *populates* the context when a valid token is present but never rejects;
  per-route `RequireAuth` / `RequireAdmin` do the rejecting. This lets one
  middleware serve both optional-auth and required-auth endpoints.
- **Indistinguishable login failures.** Unknown email and wrong password both
  return `401 invalid credentials` — the API never reveals which accounts exist.
- **The admin is seeded with no password.** The schema inserts the admin row with
  a `NULL` hash (login impossible) and a separate, out-of-band step sets the real
  password. Credentials never live in committed SQL.
- **Per-host token cache.** `bin/login` caches the JWT keyed on the server host,
  because a token signed by one environment's secret won't verify against
  another even while its `exp` is still in the future.
- **Secret from the environment, generated at setup.** `JWT_SECRET` is created
  once (`openssl rand -hex 32`) and read from config — never hardcoded, never
  committed.

---

## Implementation plan

Written Go-first (Gin + pgx + `golang-jwt/jwt/v5` + `golang.org/x/crypto/bcrypt`),
with ecosystem equivalents called out where they differ. Every phase maps onto
whatever stack you use — the shape is identical.

### Phase 0 — Pick the libraries

- **Go:** `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, your
  existing router (Gin assumed) and DB driver (pgx assumed).
- **Node:** `jsonwebtoken` + `bcrypt`/`argon2`, Express/Fastify middleware.
- **Python:** `pyjwt` + `passlib[bcrypt]`, FastAPI dependencies / Flask before_request.
- **Rust:** `jsonwebtoken` + `argon2`/`bcrypt`, axum/tower layers.

Add the deps and tidy the lockfile (`go get …@latest && go mod tidy`).

### Phase 1 — Schema: `users` table + role enum + seeded admin

Add to your DDL / a new migration. Reconcile with any existing user table rather
than creating a second one (rename columns to match the queries in Phase 3, or
adapt Phase 3's SQL to your columns).

```sql
CREATE TYPE user_role AS ENUM ('USER', 'ADMIN');

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(256),
    email VARCHAR(256) UNIQUE NOT NULL,
    passwd VARCHAR(256),                       -- bcrypt hash; NULL until set
    role USER_ROLE NOT NULL DEFAULT 'USER',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Bootstrap admin. Password is set out-of-band (Phase 8); NULL hash = no login.
INSERT INTO users (name, email, role) VALUES ('Admin', 'admin@example.com', 'ADMIN');
```

If you have a migration tool (goose/golang-migrate/Flyway/Alembic), make this a
new versioned migration so existing deploys pick it up; don't edit an applied one.

### Phase 2 — Claims + DTOs

One claims struct, shared by the issuer and the verifier so they can't drift.

```go
type JWTClaims struct {
    jwt.RegisteredClaims        // exp, iat
    UserID int64  `json:"uid"`
    Role   string `json:"role"`
}

type LoginRequest struct {
    Email    string `json:"email"    binding:"required"`
    Password string `json:"password" binding:"required"`
}

type UserDTO struct {              // never carries the password hash
    ID    int64  `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
    Role  string `json:"role"`
}

type AuthResponse struct {
    Token string  `json:"token"`
    User  UserDTO `json:"user"`
}
```

Reuse your existing `ErrorResponse` envelope if you have one; otherwise add a
small `{error, message}` type.

### Phase 3 — Repository (owns the `users` table)

Two reads, keyed by email (with hash, for login) and by id (without hash, for
profile). `COALESCE(passwd,'')` keeps the scan total when the seeded admin still
has a NULL hash; the service treats `''` as a failed credential check.

```go
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*UserDTO, string, error) {
    // SELECT id, name, email, COALESCE(passwd,''), role FROM users WHERE email=$1
    // map pgx.ErrNoRows -> ErrUserNotFound
}
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*UserDTO, error) {
    // SELECT id, name, email, role FROM users WHERE id=$1
}
```

If your project enforces single-writer table ownership, this is the file that
owns `users`.

### Phase 4 — Service (verify + issue)

```go
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
    user, hash, err := s.userRepo.GetByEmail(ctx, req.Email)
    if errors.Is(err, ErrUserNotFound) { return nil, ErrInvalidCredentials } // same error...
    if err != nil { return nil, err }
    if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
        return nil, ErrInvalidCredentials                                     // ...as wrong password
    }
    token, err := s.issueToken(user)
    if err != nil { return nil, err }
    return &AuthResponse{Token: token, User: *user}, nil
}

func (s *AuthService) issueToken(u *UserDTO) (string, error) {
    claims := JWTClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
        UserID: u.ID, Role: u.Role,
    }
    return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}
```

Registration is intentionally out of scope — provision users via the seeded admin
and direct inserts. Add a `Register` later only if you want self-service signup.

### Phase 5 — Middleware (the protect leg)

```go
// Permissive: populates context on a valid token, never rejects. Mount globally.
func ValidateUser(secret []byte) gin.HandlerFunc { /* parse Bearer, verify HS256,
    set UserIDKey/RoleKey, always c.Next() */ }

// Enforcing gates, mounted per-route:
func RequireAuth() gin.HandlerFunc  { /* 401 if UserIDKey absent */ }
func RequireAdmin() gin.HandlerFunc { /* 403 if role != "ADMIN" (mount after RequireAuth) */ }

// Context accessors:
func GetUserID(c *gin.Context) (int64, bool) { /* for optional-auth handlers */ }
func MustGetUserID(c *gin.Context) int64     { /* panics if absent — RequireAuth routes only */ }
```

Critical: `ValidateUser` must reject any token whose signing method isn't HMAC
(`if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok { return nil, ErrSignatureInvalid }`)
to block the `alg=none` / algorithm-confusion class of forgeries.

### Phase 6 — Handlers + Swagger security

`Login` binds the body and maps `ErrInvalidCredentials → 401`. `Me` is the
canonical protected endpoint — it reads `MustGetUserID(c)` and returns the
`UserDTO`. Annotate for OpenAPI and declare the Bearer scheme once, above `main()`:

```go
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

so protected handlers can carry `// @Security BearerAuth` and the Swagger UI gets
an Authorize box.

### Phase 7 — Wire into the existing router

In your current `main.go`, after the pool/config exist and the base middleware is
set up:

```go
userRepo := repository.NewUserRepository(pool)
authSvc  := services.NewAuthService(userRepo, cfg.JWTSecret)
authH    := handlers.NewAuthHandler(authSvc)

router.Use(middleware.ValidateUser([]byte(cfg.JWTSecret))) // global, permissive

v1 := router.Group("/api/v1")
a  := v1.Group("/auth")
a.POST("/login", authH.Login)
a.GET("/me", middleware.RequireAuth(), authH.Me)

// Retrofit existing routes you want protected:
//   v1.GET("/things", middleware.RequireAuth(), thingH.List)
// Admin-only group:
//   admin := v1.Group("/admin"); admin.Use(middleware.RequireAuth(), middleware.RequireAdmin())
```

Audit existing endpoints and decide, per route, whether to add `RequireAuth` —
adding the global `ValidateUser` alone changes nothing until you gate routes.

### Phase 8 — Config + env + admin bootstrap

Add to your config struct/loader and `.env(.sample)`:

```
JWT_SECRET=change-me            # setup generates: openssl rand -hex 32
ADMIN_EMAIL=admin@example.com   # MUST match the seeded row in Phase 1
ADMIN_PASS=change-me            # setup generates a random value, then sets it
API_URL=http://localhost:8080   # base URL bin/login posts to
```

In your dev-setup script, *after* the schema is applied, generate the secret and
the admin password if they're still placeholders (idempotent), then set the
admin's hash with a small helper:

```sh
# generate once
grep -q '^JWT_SECRET=change-me$' .env && sed -i "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" .env
grep -q '^ADMIN_PASS=change-me$' .env && sed -i "s|^ADMIN_PASS=.*|ADMIN_PASS=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24)|" .env
# set the admin password (bcrypt) on the seeded row
python3 bin/set_admin_password.py "$ADMIN_EMAIL" "$ADMIN_PASS" "$PG_URL"
```

`set_admin_password.py` bcrypt-hashes and `UPDATE users SET passwd=… WHERE
email=…`. For servers you can't reach directly, ship `encrypt_admin_passwd.py`
that prints the `UPDATE …` SQL to pipe into `psql "$PG_URL"` on the box. (Both
scripts are in the reference module; ~30 lines each.)

### Phase 9 — `bin/login` (mint + cache tokens)

A script that reads `ADMIN_EMAIL`/`ADMIN_PASS`/`API_URL` from `.env`, POSTs to
`/api/v1/auth/login`, caches the JWT at `~/.cache/api_token-<host-slug>` (mode
600), and reuses it until it has under 60s of validity left. It prints two
labeled lines so callers select by label, not by position:

```
TOKEN: <raw jwt>                  # TOKEN="$(bin/login | sed -n 's/^TOKEN: //p')"
SWAGGER_TOKEN: Bearer <raw jwt>   # paste into the Swagger Authorize box
```

This is what makes scripted/CI calls and the Swagger UI usable without
hand-managing tokens.

### Phase 10 — Tests (DB-backed)

Against an ephemeral test database with the schema applied and the admin seeded:

1. Seed a known bcrypt password on the admin row, then assert `POST /auth/login`
   returns 200 + a non-empty token whose `UserDTO` has `role=ADMIN`.
2. Wrong password and unknown email both return **401**.
3. A protected route with no token returns **401**; with the admin token, **2xx**.
4. Insert a `USER`-role row, mint its token, and assert an admin-only route
   returns **403**.

Drive these through an in-process router (`httptest` + the real middleware +
handlers) so the full parse→gate→handler path is exercised, not just the service.

---

## What a repo gets from this

- **Working login + route protection** with one secret and one table — no IdP to
  stand up, no third-party round-trips.
- **Fail-closed gating:** a protected route can't silently serve an anonymous
  request; misconfiguration surfaces as a 500, not a bypass.
- **Forgery-resistant tokens:** HMAC-only verification blocks `alg`-confusion
  attacks; per-host caching prevents cross-environment token reuse.
- **Scriptable auth:** `bin/login` gives CI, curl, and Swagger a one-command token.
- **A clean upgrade path:** moving to an external OAuth/OIDC provider later
  replaces only the *issue* and *verify-key* steps; the middleware, route gating,
  and `users` model stay exactly as they are.
```
