// SPDX-License-Identifier: MIT

package services

import (
	"context"
	"errors"
	"time"

	"example.com/app/optional/auth/internal/models"
	"example.com/app/optional/auth/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// tokenTTL is how long an issued JWT stays valid. bin/login caches a token until
// it has under 60s left, so keep this comfortably above a single CLI session.
const tokenTTL = 24 * time.Hour

// ErrInvalidCredentials is returned for both an unknown email and a wrong
// password, so the API never reveals which one was wrong.
var ErrInvalidCredentials = errors.New("invalid credentials")

type AuthService struct {
	userRepo  *repository.UserRepository
	jwtSecret []byte
}

func NewAuthService(userRepo *repository.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{userRepo: userRepo, jwtSecret: []byte(jwtSecret)}
}

// Login verifies credentials and returns a signed JWT on success.
// Returns ErrInvalidCredentials for an unknown email or a wrong password.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {
	user, hash, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.issueToken(user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{Token: token, User: *user}, nil
}

// GetUserByID returns the UserDTO for the given user ID.
func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*models.UserDTO, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *AuthService) issueToken(user *models.UserDTO) (string, error) {
	claims := models.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID: user.ID,
		Role:   user.Role,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}
