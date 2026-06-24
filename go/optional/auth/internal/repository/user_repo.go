// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"

	"example.com/app/optional/auth/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned when a lookup matches no row. The auth service
// maps it to ErrInvalidCredentials on login so the API never reveals whether an
// email exists.
var ErrUserNotFound = errors.New("user not found")

// UserRepository owns the `users` table. Per the kit's repository-ownership gate
// (tests/quality_test.go), only this file may read/write `users`; other modules
// call these methods rather than querying the table directly.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// GetByEmail returns the UserDTO and the stored password hash for the given email.
// COALESCE keeps the scan total when passwd is NULL (a seeded admin before its
// password has been set), which the service treats as a failed credential check.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.UserDTO, string, error) {
	var u models.UserDTO
	var hash string
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(passwd, ''), role
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Name, &u.Email, &hash, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrUserNotFound
		}
		return nil, "", err
	}
	return &u, hash, nil
}

// GetByID returns the UserDTO for the given user ID (never the password hash).
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*models.UserDTO, error) {
	var u models.UserDTO
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, email, role
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
