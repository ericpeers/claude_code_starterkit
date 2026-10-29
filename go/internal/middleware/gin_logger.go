// SPDX-License-Identifier: MIT

// Package middleware holds request-scoped Gin middleware. GinLogger routes Gin's
// request lines through logrus, so every log line shares one format and level.
// Build the router with gin.New(), since gin.Default() adds a second logger:
//
//	router.Use(gin.Recovery(), middleware.GinLogger())
package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// GinLogger returns Gin middleware that logs each completed request through
// logrus. The level follows the status: 5xx Error, 4xx Warn, else Info.
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
