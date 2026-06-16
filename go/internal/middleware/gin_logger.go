// SPDX-License-Identifier: MIT

// Package middleware holds request-scoped Gin middleware. GinLogger here is the
// kit's unified-logging building block: it routes Gin's per-request log lines
// through logrus so every line in the app — startup, request, and application
// logs — shares one format, timestamp layout, and level filter. Wire it in with
// gin.New() (NOT gin.Default(), which installs Gin's own stdout logger):
//
//	router := gin.New()
//	router.Use(gin.Recovery(), middleware.GinLogger())
//
// and configure logrus once at startup (see CLAUDE.md "Logging" for the full
// pattern). Generated docs and other internal/ middleware live alongside this.
package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// GinLogger returns Gin middleware that logs each completed request through
// logrus, choosing the level from the response status: 5xx -> Error, 4xx ->
// Warn, everything else -> Info. Because it logs via logrus, the LOGLEVEL set at
// startup also filters request logs (e.g. LOGLEVEL=error hides 2xx/3xx noise).
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		status := c.Writer.Status()
		msg := fmt.Sprintf("%s %s | %d | %s | %s",
			c.Request.Method,
			c.Request.URL.Path,
			status,
			duration.Round(time.Millisecond),
			c.ClientIP(),
		)

		switch {
		case status >= 500:
			log.Error(msg)
		case status >= 400:
			log.Warn(msg)
		default:
			log.Info(msg)
		}
	}
}
