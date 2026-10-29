// SPDX-License-Identifier: MIT

package tests

// Length and placement gates for Go comments, from the Budget and API doc
// budget rules in CONVENTIONS.md. They use the style gates' baseline mechanism.

import (
	"fmt"
	"go/ast"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// maxCommentBlockLines is the ceiling on one contiguous comment block. A longer
// block is rationale that belongs in docs/.
const maxCommentBlockLines = 6

// A function doc gets maxFuncDocLines prose lines, or maxOneLineFuncDocLines
// when the body is one line. Indented lines, such as list items, do not count.
const (
	maxFuncDocLines        = 4
	maxOneLineFuncDocLines = 2
)

// maxSwaggerDescriptionSentences is the sentence budget of one route's @Description.
const maxSwaggerDescriptionSentences = 3

// maxTrailingCommentWidth is the widest line a field comment may make when it
// moves to the end of the field's line.
const maxTrailingCommentWidth = 120

// maxFoldedCommentLines is the longest field doc the gate asks to fold onto the
// field's line. A longer one needs trimming first.
const maxFoldedCommentLines = 2

// The two placements the field gate reports, used as the hit's reason.
const (
	reasonFitsOnLine = "fits at the end of the field's line"
	reasonPeersTrail = "peers put their comment at the end of the line"
)

// swaggerSentenceEnd matches the punctuation that ends a sentence.
var swaggerSentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

var (
	commentBlockGuard = styleGuard{
		name:     "long comment blocks",
		baseline: "comment_blocks.txt",
		fix:      "move the rationale into docs/ and leave a one-line pointer",
	}
	funcDocGuard = styleGuard{
		name:     "long function docs",
		baseline: "func_docs.txt",
		fix:      "cut the doc to what the name does not say (see Budget in CONVENTIONS.md)",
	}
	swaggerDescriptionGuard = styleGuard{
		name:     "long swagger descriptions",
		baseline: "swagger_descriptions.txt",
		fix:      "move field meanings to the model and error cases to @Failure lines",
	}
	fieldCommentGuard = styleGuard{
		name:     "field comment placement",
		baseline: "field_comments.txt",
		fix:      "move the comment to the end of the field's line, or trim it",
	}
)

// commentLineCount is how many lines a comment group covers, less directive lines.
func commentLineCount(group *ast.CommentGroup) int {
	lines := 0
	for _, comment := range group.List {
		if goDirective.MatchString(comment.Text) {
			continue
		}
		lines += strings.Count(comment.Text, "\n") + 1
	}
	return lines
}

// docProseLineCount is how many prose lines a doc comment holds. Blank,
// indented, and directive lines are skipped.
func docProseLineCount(doc *ast.CommentGroup) int {
	lines := 0
	for _, comment := range doc.List {
		for _, line := range strings.Split(comment.Text, "\n") {
			if goDirective.MatchString(line) {
				continue
			}
			body := strings.TrimPrefix(line, "//")
			if strings.TrimSpace(body) == "" || strings.HasPrefix(body, "\t") || strings.HasPrefix(body, "  ") {
				continue
			}
			lines++
		}
	}
	return lines
}

// commentBlockHits reports each comment block over maxCommentBlockLines. The
// license header on line 1 is left out.
func commentBlockHits(file goFile) []styleHit {
	var hits []styleHit
	for _, group := range file.syntax.Comments {
		if isLicenseHeader(file.fset, file.syntax, group) {
			continue
		}
		start := file.fset.Position(group.Pos()).Line
		if lines := commentLineCount(group); lines > maxCommentBlockLines {
			hits = append(hits, styleHit{start, fmt.Sprintf("%d-line block, limit %d", lines, maxCommentBlockLines),
				group.Text()})
		}
	}
	return hits
}

// funcDocHits reports each function whose doc comment runs over its limit.
func funcDocHits(file goFile) []styleHit {
	var hits []styleHit
	for _, decl := range file.syntax.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil || fn.Body == nil {
			continue
		}
		limit := maxFuncDocLines
		// The braces sit on their own lines, so a one-line body spans two.
		if file.fset.Position(fn.Body.Rbrace).Line-file.fset.Position(fn.Body.Lbrace).Line <= 2 {
			limit = maxOneLineFuncDocLines
		}
		if lines := docProseLineCount(fn.Doc); lines > limit {
			hits = append(hits, styleHit{file.fset.Position(fn.Doc.Pos()).Line,
				fmt.Sprintf("%d-line doc, limit %d", lines, limit), fn.Name.Name + ": " + fn.Doc.Text()})
		}
	}
	return hits
}

// swaggerDescriptionHits reports each function whose @Description runs over
// maxSwaggerDescriptionSentences.
func swaggerDescriptionHits(file goFile) []styleHit {
	var hits []styleHit
	for _, decl := range file.syntax.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		var description []string
		for _, comment := range fn.Doc.List {
			if text, found := strings.CutPrefix(comment.Text, "// @Description"); found {
				description = append(description, strings.TrimSpace(text))
			}
		}
		joined := strings.Join(description, " ")
		if sentences := len(swaggerSentenceEnd.FindAllString(joined, -1)); sentences > maxSwaggerDescriptionSentences {
			hits = append(hits, styleHit{file.fset.Position(fn.Doc.Pos()).Line,
				fmt.Sprintf("%d sentences, limit %d", sentences, maxSwaggerDescriptionSentences), fn.Name.Name + ": " + joined})
		}
	}
	return hits
}

// fieldDocText joins a field's doc comment into one line, without markers. It
// returns "" unless the comment opens with the field's name. This skips a
// section header, which go/ast attaches to the next field.
func fieldDocText(name string, doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	joined := strings.Join(strings.Fields(doc.Text()), " ")
	if !strings.HasPrefix(joined, name+" ") {
		return ""
	}
	return joined
}

// trailingWidth is the field line's width with the comment appended. An
// out-of-range line reports one past the limit.
func trailingWidth(src []string, line int, text string) int {
	if line < 1 || line > len(src) {
		return maxTrailingCommentWidth + 1
	}
	return displayWidth(strings.TrimRight(src[line-1], " \t")) + len(" // ") + utf8.RuneCountInString(text)
}

// fieldCommentHits reports each struct field whose comment sits above it but
// belongs at the end of its line. Either it fits there, or its peers trail theirs.
func fieldCommentHits(file goFile) []styleHit {
	var hits []styleHit
	// The closure appends to hits, which ast.Inspect has no way to return.
	ast.Inspect(file.syntax, func(node ast.Node) bool {
		structType, ok := node.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return true
		}
		hits = append(hits, structFieldHits(file, structType.Fields.List)...)
		return true
	})
	return hits
}

// structFieldHits applies the placement rule to the fields of one struct.
func structFieldHits(file goFile, fields []*ast.Field) []styleHit {
	trailingPeers := false
	for _, field := range fields {
		trailingPeers = trailingPeers || field.Comment != nil
	}
	var hits []styleHit
	for _, field := range fields {
		if len(field.Names) != 1 || field.Comment != nil {
			continue
		}
		text := fieldDocText(field.Names[0].Name, field.Doc)
		if text == "" {
			continue
		}
		declLine := file.fset.Position(field.Pos()).Line
		reason := ""
		switch {
		case commentLineCount(field.Doc) <= maxFoldedCommentLines &&
			trailingWidth(file.lines, declLine, text) <= maxTrailingCommentWidth:
			reason = reasonFitsOnLine
		case trailingPeers:
			reason = reasonPeersTrail
		default:
			continue
		}
		hits = append(hits, styleHit{file.fset.Position(field.Doc.Pos()).Line, reason,
			field.Names[0].Name + ": " + text})
	}
	return hits
}

// collectGoHits runs one detector over every Go file and groups the hits by file.
func collectGoHits(files []goFile, detect func(goFile) []styleHit) map[string][]styleHit {
	perFile := map[string][]styleHit{}
	for _, file := range files {
		if hits := detect(file); len(hits) > 0 {
			perFile[file.rel] = hits
		}
	}
	return perFile
}

// TestCommentBlockLength fails on a comment block over maxCommentBlockLines in
// Go source, tests included.
func TestCommentBlockLength(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadGoFiles(t, root)
	runStyleGuard(t, root, commentBlockGuard, loadProseFiles(t, root), collectGoHits(files, commentBlockHits))
}

// TestFuncDocLength fails on a function doc comment over its limit, tests included.
func TestFuncDocLength(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadGoFiles(t, root)
	runStyleGuard(t, root, funcDocGuard, loadProseFiles(t, root), collectGoHits(files, funcDocHits))
}

// TestSwaggerDescriptionLength keeps each route's @Description to one to three sentences.
func TestSwaggerDescriptionLength(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := productionFiles(loadGoFiles(t, root))
	runStyleGuard(t, root, swaggerDescriptionGuard, loadProseFiles(t, root), collectGoHits(files, swaggerDescriptionHits))
}

// TestFieldCommentPlacement keeps a struct field's comment at the end of its
// line when it fits there, and one struct's fields consistent with each other.
func TestFieldCommentPlacement(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := productionFiles(loadGoFiles(t, root))
	runStyleGuard(t, root, fieldCommentGuard, loadProseFiles(t, root), collectGoHits(files, fieldCommentHits))
}

// commentLengthFixture seeds violations for each length gate, beside source
// each gate passes.
const commentLengthFixture = `package sample

// longBlock opens a seven-line block.
// two
// three
// four
// five
// six
// seven
var longBlock = 1

// shortBlock is within budget.
var shortBlock = 2

// longDoc has a five-line doc.
// two
// three
// four
// five
func longDoc() int {
	x := 1
	return x
}

// oneLiner has a three-line doc.
// two
// three
func oneLiner() int { return 1 }

// listDoc counts its list items as zero lines:
//   - first
//   - second
//   - third
func listDoc() int { return 1 }

// ListOrders lists orders.
// @Description Lists orders. Filters by status. Sorts by date. Pages the result.
func ListOrders() {}

// GetOrder returns one order.
// @Description Returns one order. Includes its line items.
func GetOrder() {}
`

// TestCommentLengthGatesFindSeededHits fails when a length detector misses a
// seeded violation or flags clean source.
func TestCommentLengthGatesFindSeededHits(t *testing.T) {
	t.Parallel()
	file, err := parseGoSource("sample.go", "sample.go", commentLengthFixture)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		detect func(goFile) []styleHit
		lines  []int
	}{
		{"comment blocks", commentBlockHits, []int{3}},
		{"function docs", funcDocHits, []int{15, 25}},
		{"swagger descriptions", swaggerDescriptionHits, []int{36}},
	}
	for _, tc := range cases {
		hits := tc.detect(file)
		var got []int
		for _, hit := range hits {
			got = append(got, hit.line)
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.lines) {
			t.Errorf("%s: hits on lines %v, want %v (%+v)", tc.name, got, tc.lines, hits)
		}
	}
}

// fieldCommentCases are fixtures for the placement detector. Three are reported:
// a comment that folds, a block comment that folds, and a field out of step
// with its peers. A section header and an unfoldable comment are left alone.
var fieldCommentCases = []struct {
	name string
	src  string
	want []styleHit
}{
	{
		name: "short comment folds onto the field line",
		src:  "package sample\n\ntype Page struct {\n\t// Offset is the pagination offset of this page.\n\tOffset int\n}\n",
		want: []styleHit{{4, reasonFitsOnLine, "Offset: Offset is the pagination offset of this page."}},
	},
	{
		name: "block comment folds without its closing marker",
		src:  "package sample\n\ntype Page struct {\n\t/* Offset is the pagination offset of this page. */\n\tOffset int\n}\n",
		want: []styleHit{{4, reasonFitsOnLine, "Offset: Offset is the pagination offset of this page."}},
	},
	{
		name: "long comment is reported when peers trail theirs",
		src: "package sample\n\ntype LineItem struct {\n\tID int // the line item\n" +
			"\t// Status is the fulfilment state of the line item. It moves from pending to picked to\n" +
			"\t// shipped, and a cancelled order moves every one of its line items to cancelled in one step.\n" +
			"\t// A shipped line item keeps its status after the order closes.\n" +
			"\tStatus string\n}\n",
		want: []styleHit{{5, reasonPeersTrail, "Status: Status is the fulfilment state of the line item. " +
			"It moves from pending to picked to shipped, and a cancelled order moves every one of its line " +
			"items to cancelled in one step. A shipped line item keeps its status after the order closes."}},
	},
	{
		name: "section header above a run of fields is left alone",
		src:  "package sample\n\ntype Order struct {\n\tID int // the order\n\n\t// shipping fields\n\tAddress string\n}\n",
	},
	{
		name: "long comment stands when no peer trails one",
		src: "package sample\n\ntype User struct {\n" +
			"\t// Email is the address the user signs in with, stored lower case with surrounding spaces removed.\n" +
			"\t// Two users may not share one.\n\tEmail string\n}\n",
	},
}

// TestFieldCommentHitsFixtures fails when the placement detector misreports a fixture.
func TestFieldCommentHitsFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range fieldCommentCases {
		file, err := parseGoSource("sample.go", "sample.go", tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := fieldCommentHits(file); fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", tc.name, got, tc.want)
		}
	}
}
