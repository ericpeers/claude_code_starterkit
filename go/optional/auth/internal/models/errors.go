// SPDX-License-Identifier: MIT

package models

// ErrorResponse is the standard JSON error envelope returned by the auth
// handlers and middleware. It is intentionally generic. If your generated app
// already defines an ErrorResponse type, delete this file and let the existing
// one win (the handlers/middleware reference models.ErrorResponse by name).
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
