// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"go/ast"
	"strings"
)

// region Verbatim comments

// isVerbatim reports whether a comment style is emitted exactly as it was written.
//
// The four styles below already say "Verbatim" where they are declared, and [SourceFile.Cleanup] already
// honors that by declining to style them. [SourceFile.SaveAs] did not: it sent every [CommentDecl] through
// renderDoc, which is go/doc/comment's prose printer, and a prose printer reflows. This is the same rule
// stated once more on the way out (#994).
//
// Parameters:
//   - `style`: the comment's classification.
//
// Returns:
//   - `bool`: true when the comment is emitted as written rather than rendered.
func isVerbatim(style CommentStyle) bool {

	switch style {
	case StyleCopyright, StyleDelineator, StyleRegionMarker, StyleSectionHeader:
		return true
	case StylePackageDoc, StyleImportDoc, StyleProse:
		return false
	default:
		return false
	}
}

// renderCommentDecl emits a standalone comment, rendering prose and copying everything else.
//
// A styler asked to add a doc comment to a declaration leaves every other byte of the file alone. For a
// verbatim style there is nothing to render: the bytes on disk are the answer, and the parsed [comment.Doc]
// is a lossy view of them. An SPDX header is the case that proved it -- two adjacent comment lines are one
// paragraph to go/doc/comment, and a paragraph that fits the width budget comes back as one line, so
// `SPDX-License-Identifier: Apache-2.0` became `Apache-2.0 Copyright Noble Factor. All rights reserved.`
// and no SPDX consumer could resolve it. 848 files were in scope for the sweep this was about to drive.
//
// Parameters:
//   - `cd`: the standalone comment declaration.
//   - `width`: the line-width budget, used only for prose.
//
// Returns:
//   - `string`: the comment block, without a trailing newline.
func renderCommentDecl(cd *CommentDecl, width int) string {

	if cd == nil {
		return ""
	}

	if isVerbatim(cd.style) {
		return renderVerbatim(cd.cg)
	}

	return renderDoc(cd.doc, width)
}

// renderVerbatim returns a comment group's lines exactly as they were parsed.
//
// [ast.Comment.Text] is the field holding the raw line, markers and all, not the [ast.CommentGroup.Text]
// method that strips them -- stripping is what the caller is being protected from.
//
// Parameters:
//   - `cg`: the comment group, or nil.
//
// Returns:
//   - `string`: the original lines joined by newlines, or the empty string when `cg` is nil or empty.
func renderVerbatim(cg *ast.CommentGroup) string {

	if cg == nil || len(cg.List) == 0 {
		return ""
	}

	lines := make([]string, 0, len(cg.List))

	for _, line := range cg.List {
		lines = append(lines, line.Text)
	}

	return strings.Join(lines, "\n")
}

// endregion
