// SPDX-License-Identifier: MIT

package tests

// Three gates for the Comment style rules in CONVENTIONS.md. The baselines and
// the two environment variables are documented in CLAUDE_go.md under Testing.
// getRepoRoot resolves to the enclosing Go module, so the gates cover that
// module. They cover the whole repository only when go.mod sits at its top.

import "testing"

// TestNoBannedCommentWords enforces the vocabulary ban list over Go, SQL, and
// Python comments and over Markdown prose.
func TestNoBannedCommentWords(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, bannedWordGuard, files, collectWordHits(files, bannedCommentWords))
}

// TestNoAbsoluteClaims enforces the No absolute claims rule over the same files.
// Instruction documents are left out, since the words read as orders there.
func TestNoAbsoluteClaims(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, absoluteClaimGuard, files, collectWordHits(files, absoluteClaimWords))
}

// TestCommentSentenceLength flags sentences over maxSentenceWords. A long
// sentence is a clause chain to split.
func TestCommentSentenceLength(t *testing.T) {
	t.Parallel()
	root := getRepoRoot(t)
	files := loadProseFiles(t, root)
	runStyleGuard(t, root, sentenceLengthGuard, files, collectLongSentences(files))
}
