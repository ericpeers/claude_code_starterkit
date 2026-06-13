// SPDX-License-Identifier: MIT

package tests

// API naming conventions enforced against the generated Swagger docs:
//
//   - URL path segments          -> kebab-case  (lowercase, hyphen-separated)
//   - JSON payload fields         -> snake_case  (lowercase, underscore-separated)
//   - Query / body / form params  -> snake_case
//   - Path template params ({user_id}) -> snake_case (they bind to identifiers)
//
// Rationale: hyphens are the convention for URLs (case-insensitive, hyphen-safe
// in DNS and readable), while payload keys are accessed as code identifiers where
// hyphens are illegal, so they follow the snake_case used by DB columns and Go
// json tags. Locking this into a test keeps the surface consistent as new
// endpoints land.
//
// File-based gate: skips gracefully when docs/swagger.json is absent, so a fresh
// scaffold stays green until you generate Swagger docs (e.g. via `swag init`).
//
// Customize:
//   - bodyWrappers : generic whole-struct body parameter names to exempt.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// isKebabCase reports whether s is a single lowercase word or hyphen-separated
// lowercase words (e.g. "sample-csv", "version"). Rejects underscores and
// uppercase letters.
func isKebabCase(s string) bool {
	if s == "" {
		return false
	}
	prevHyphen := true // disallow leading hyphen
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			prevHyphen = false
		case r >= '0' && r <= '9':
			// digits are fine, but not as the very first character
			if i == 0 {
				return false
			}
			prevHyphen = false
		case r == '-':
			if prevHyphen { // leading or doubled hyphen
				return false
			}
			prevHyphen = true
		default:
			return false
		}
	}
	return !prevHyphen // disallow trailing hyphen
}

// isSnakeCase reports whether s is a single lowercase word or underscore-separated
// lowercase words (e.g. "user_id", "q"). Rejects hyphens and uppercase letters.
func isSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	prevUnderscore := true // disallow leading underscore
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			prevUnderscore = false
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
			prevUnderscore = false
		case r == '_':
			if prevUnderscore { // leading or doubled underscore
				return false
			}
			prevUnderscore = true
		default:
			return false
		}
	}
	return !prevUnderscore // disallow trailing underscore
}

type swaggerDoc struct {
	Paths       map[string]map[string]swaggerOperation `json:"paths"`
	Definitions map[string]swaggerDefinition           `json:"definitions"`
}

type swaggerOperation struct {
	Parameters []swaggerParameter `json:"parameters"`
}

type swaggerParameter struct {
	Name string `json:"name"`
	In   string `json:"in"`
}

type swaggerDefinition struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

// loadSwaggerDoc reads and parses docs/swagger.json. It returns ok=false (and
// the caller should t.Skip) when the file does not exist yet, so a fresh
// scaffold without generated docs still passes.
func loadSwaggerDoc(t *testing.T) (swaggerDoc, bool) {
	t.Helper()
	path := optionalFilePath(t, "docs/swagger.json")
	if path == "" {
		return swaggerDoc{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading swagger.json: %v", err)
	}
	var doc swaggerDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing swagger.json: %v", err)
	}
	return doc, true
}

// TestSwaggerURLsAreKebabCase asserts every static URL path segment uses
// kebab-case. Template parameters ({user_id}) are validated as snake_case
// separately because they bind to identifiers, not URL words.
func TestSwaggerURLsAreKebabCase(t *testing.T) {
	t.Parallel()
	doc, ok := loadSwaggerDoc(t)
	if !ok {
		t.Skip("docs/swagger.json not present yet; nothing to check")
	}

	for path := range doc.Paths {
		for segment := range strings.SplitSeq(path, "/") {
			if segment == "" {
				continue
			}
			if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
				name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
				if !isSnakeCase(name) {
					t.Errorf("path %q: template param {%s} is not snake_case", path, name)
				}
				continue
			}
			if !isKebabCase(segment) {
				t.Errorf("path %q: segment %q is not kebab-case (use hyphens, not underscores)", path, segment)
			}
		}
	}
}

// TestSwaggerJSONFieldsAreSnakeCase asserts every JSON payload field name in the
// model definitions uses snake_case.
func TestSwaggerJSONFieldsAreSnakeCase(t *testing.T) {
	t.Parallel()
	doc, ok := loadSwaggerDoc(t)
	if !ok {
		t.Skip("docs/swagger.json not present yet; nothing to check")
	}

	for defName, def := range doc.Definitions {
		for field := range def.Properties {
			if !isSnakeCase(field) {
				t.Errorf("definition %s: field %q is not snake_case", defName, field)
			}
		}
	}
}

// TestSwaggerParamsAreSnakeCase asserts query, body, and form parameter names use
// snake_case. The generic "body"/"request" wrapper names that swag emits for
// whole-struct request bodies are exempt — they are not part of the wire
// contract, only the doc's internal parameter label. Add your own whole-struct
// body wrapper names to bodyWrappers as needed.
func TestSwaggerParamsAreSnakeCase(t *testing.T) {
	t.Parallel()
	doc, ok := loadSwaggerDoc(t)
	if !ok {
		t.Skip("docs/swagger.json not present yet; nothing to check")
	}

	bodyWrappers := map[string]bool{"body": true, "request": true}

	for path, methods := range doc.Paths {
		for method, op := range methods {
			for _, p := range op.Parameters {
				if p.In == "body" && bodyWrappers[p.Name] {
					continue
				}
				if !isSnakeCase(p.Name) {
					t.Errorf("%s %s: %s param %q is not snake_case", strings.ToUpper(method), path, p.In, p.Name)
				}
			}
		}
	}
}
