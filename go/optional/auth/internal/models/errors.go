// SPDX-License-Identifier: MIT

package models

// ErrorResponse is the JSON error envelope the auth handlers return. Delete this
// file when your app already defines the type. Handlers and middleware refer to
// it as models.ErrorResponse.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
