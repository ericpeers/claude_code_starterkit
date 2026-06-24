// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"net/http"

	"example.com/app/optional/auth/internal/middleware"
	"example.com/app/optional/auth/internal/models"
	"example.com/app/optional/auth/internal/repository"
	"example.com/app/optional/auth/internal/services"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type AuthHandler struct {
	authSvc *services.AuthService
}

func NewAuthHandler(authSvc *services.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

// Login authenticates a user and returns a signed JWT on success.
// @Summary Log in
// @Description Verifies credentials and returns a Bearer token.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.LoginRequest true "Email and password"
// @Success 200 {object} models.AuthResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse "Invalid credentials"
// @Failure 500 {object} models.ErrorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "bad_request", Message: err.Error()})
		return
	}

	resp, err := h.authSvc.Login(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{Error: "unauthorized", Message: "invalid email or password"})
			return
		}
		// Log the underlying cause server-side but return a generic message so we
		// don't leak internal/DB error detail to the client.
		log.Errorf("Login: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "internal_error", Message: "internal error"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Me returns the profile of the currently authenticated user.
// @Summary Get current user
// @Description Returns the UserDTO for the user identified by the Bearer token.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.UserDTO
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	// Mounted behind RequireAuth, so an authenticated user is guaranteed present. Using
	// MustGetUserID fails closed rather than querying user 0 if auth were ever absent.
	userID := middleware.MustGetUserID(c)
	user, err := h.authSvc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{Error: "not_found", Message: "user not found"})
			return
		}
		log.Errorf("Me: lookup user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "internal_error", Message: "internal error"})
		return
	}
	c.JSON(http.StatusOK, user)
}
