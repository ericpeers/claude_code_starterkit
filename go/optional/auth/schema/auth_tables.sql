-- SPDX-License-Identifier: MIT
-- Auth schema for the optional auth module: a single `users` table plus the role
-- enum. install.sh copies this to create_tables.sql when you don't have one yet;
-- otherwise merge these statements into your existing create_tables.sql.
--
-- Keep this DDL free of `IF NOT EXISTS`, which CREATE TYPE has no form for.
-- setup_dev.sh applies create_tables.sql only to an empty public schema, so a
-- re-run does not reapply the DDL.

CREATE TYPE user_role AS ENUM ('USER', 'ADMIN');

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(256),
    email VARCHAR(256) UNIQUE NOT NULL,
    passwd VARCHAR(256),
    role USER_ROLE NOT NULL DEFAULT 'USER',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Bootstrap admin. The password is set after the schema is applied, by
-- scripts/setup_admin.sh (which calls bin/set_admin_password.py). Until then
-- passwd is NULL and login is refused, so the seeded row is inert.
INSERT INTO users (name, email, role) VALUES ('Admin', 'admin@example.com', 'ADMIN');
