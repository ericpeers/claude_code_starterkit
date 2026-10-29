// SPDX-License-Identifier: MIT

// Package errortext holds handlers for the error-text gate's self-test. Each
// reply marked "leak" sends error text to the client.
package errortext

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

var errNotFound = errors.New("order 42 not found in orders")

// lookupOrder fails for an empty id.
func lookupOrder(id string) error {
	if id == "" {
		return errNotFound
	}
	return nil
}

// describe formats the error for a reply.
func describe(err error) string {
	return "lookup failed: " + err.Error()
}

// lookupMessage returns the lookup error's text, or "" on success.
func lookupMessage(id string) string {
	if err := lookupOrder(id); err != nil {
		return err.Error()
	}
	return ""
}

// codedError is an error whose Code is written for the client.
type codedError struct{ Code string }

func (e *codedError) Error() string { return "order lookup failed with " + e.Code }

// encoder writes to a log. Its JSON method shares a name with gin's.
type encoder struct{}

func (encoder) JSON(code int, body any) {}

// replyError sends msg as the error body.
func replyError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg}) // leak
}

// LeakDirect replies with the error's text.
func LeakDirect(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}) // leak
	}
}

// LeakFormatted formats the error into the reply.
func LeakFormatted(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("lookup failed: %v", err)) // leak
	}
}

// LeakThroughLocal builds the reply text in a local first.
func LeakThroughLocal(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		msg := "lookup failed: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg}) // leak
	}
}

// LeakThroughHelper gets the reply text from a helper.
func LeakThroughHelper(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": describe(err)}) // leak
	}
}

// ReplyFixedText sends text written for the caller.
func ReplyFixedText(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		msg := "order not found"
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
	}
}

// LeakThroughReturn replies with text a helper built from an error.
func LeakThroughReturn(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": lookupMessage(c.Param("id"))}) // leak
}

// LeakThroughReturnAgain reaches the same helper from a second reply.
func LeakThroughReturnAgain(c *gin.Context) {
	c.String(http.StatusInternalServerError, lookupMessage(c.Query("id"))) // leak
}

// LeakThroughParameter passes error text to a reply helper.
func LeakThroughParameter(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		replyError(c, err.Error())
	}
}

// LeakThroughVarDecl declares the reply text with var.
func LeakThroughVarDecl(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		var msg = "lookup failed: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg}) // leak
	}
}

// LeakThroughRange ranges over a slice that holds error text.
func LeakThroughRange(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		for _, part := range []string{err.Error()} {
			c.String(http.StatusInternalServerError, part) // leak
		}
	}
}

// ReplyErrorCode sends the Code field of a typed error.
func ReplyErrorCode(c *gin.Context) {
	var coded *codedError
	if err := lookupOrder(c.Param("id")); errors.As(err, &coded) {
		c.JSON(http.StatusBadRequest, gin.H{"code": coded.Code})
	}
}

// LogOnly sends error text to an encoder that is not gin's.
func LogOnly(c *gin.Context) {
	if err := lookupOrder(c.Param("id")); err != nil {
		encoder{}.JSON(http.StatusInternalServerError, err.Error())
	}
}
