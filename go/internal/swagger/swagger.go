// SPDX-License-Identifier: MIT

// Package swagger wires the Swagger UI into a Gin router. It is the kit's
// swagger building block: keep the OpenAPI surface generated from handler
// annotations (one source of truth) rather than hand-maintained.
//
// Workflow:
//  1. Annotate handlers and main() with swag comments (see CLAUDE.md "Swagger").
//  2. Generate the spec:  make docs   (runs `swag init`, writes ./docs).
//  3. Blank-import the generated package in main.go so its init() registers the
//     spec:  import _ "<your-module>/docs"
//  4. Mount the UI behind the ENABLE_SWAGGER flag:  swagger.Register(router)
//
// TestSwagger* in tests/ then lint the generated docs/swagger.json for the kit's
// URL/field naming conventions.
package swagger

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Register mounts the Swagger UI at /swagger/index.html on r. Gate the call on
// your ENABLE_SWAGGER config so the UI is off in production. The served spec
// comes from the generated docs package blank-imported in main.go; without that
// import the UI loads but reports a missing doc.json.
func Register(r gin.IRouter) {
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
