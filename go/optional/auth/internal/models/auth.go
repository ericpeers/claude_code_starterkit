// SPDX-License-Identifier: MIT

package models

import "github.com/golang-jwt/jwt/v5"

// Role values. These mirror the `user_role` enum in schema/auth_tables.sql —
// keep the two in lockstep. The middleware gates on RoleAdmin; the service and
// JWT claims carry whichever role the user row holds.
const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"
)

// JWTClaims is the shared claims struct used by both the auth service (token
// creation) and the auth middleware (token parsing). Keep the two in lockstep:
// the middleware parses exactly what issueToken signed.
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
}

// LoginRequest is the body of POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UserDTO is the public representation of a user. It never carries the password
// hash, so it is safe to embed in responses and JWT-adjacent payloads.
type UserDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// AuthResponse is returned on a successful login.
type AuthResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}
