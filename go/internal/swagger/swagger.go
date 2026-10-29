// SPDX-License-Identifier: MIT

// Package swagger mounts the Swagger UI on a Gin router. The spec is generated
// from handler annotations by `make docs`. The workflow is in CLAUDE.md under
// Swagger / API docs.
package swagger

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Register mounts the Swagger UI at /swagger/index.html. Call it only when
// ENABLE_SWAGGER is set, and blank-import the generated docs package in main.go.
func Register(r gin.IRouter) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
