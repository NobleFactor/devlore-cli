// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

// verbatimSource carries one comment of each style the [CommentStyle] constants call verbatim, so a reflow
// anywhere in [SourceFile.SaveAs] shows up as a difference rather than as a passing test.
const verbatimSource = `// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package example

// region EXPORTED METHODS

// =============================================================================

// Fallible actions

// Named returns the name.
//
// Returns:
//   - ` + "`string`" + `: the name.
func Named() string { return "n" }

// endregion
`

// TestSaveAsKeepsVerbatimCommentsVerbatim pins the contract the [CommentStyle] constants already state and
// [SourceFile.SaveAs] did not honor.
//
// Four styles are documented "Verbatim" -- copyright, delineator, region marker and section header -- and
// [SourceFile.Cleanup] respects that, declining to style any of them. SaveAs then sent every [CommentDecl]
// through renderDoc regardless of style, and renderDoc is go/doc/comment's prose printer: consecutive
// non-blank lines are one paragraph, and a paragraph is reflowed to the width budget. The two-line SPDX
// header fits well inside 120 columns, so it came back as ONE line and the license identifier became
// `Apache-2.0 Copyright Noble Factor. All rights reserved.`, which no SPDX consumer resolves (#994).
//
// 848 files are in scope for the go-style sweep, so one `--fix` run over the tree would have rewritten 848
// license headers into invalid ones.
//
// The assertion is deliberately the general one rather than a check for that header: a styler asked to add
// a doc comment to a declaration leaves every other byte alone. Naming only SPDX would leave the three other
// verbatim styles free to reflow.
func TestSaveAsKeepsVerbatimCommentsVerbatim(t *testing.T) {

	for _, testCase := range []struct {
		name string
		want string
	}{
		{
			name: "the SPDX header stays two lines",
			want: "// SPDX-License-Identifier: Apache-2.0\n// Copyright Noble Factor. All rights reserved.",
		},
		{
			name: "the region marker survives",
			want: "// region EXPORTED METHODS",
		},
		{
			name: "the delineator survives",
			want: "// =============================================================================",
		},
		{
			name: "the section header survives",
			want: "// Fallible actions",
		},
		{
			name: "the endregion marker survives",
			want: "// endregion",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got := cleanupAndSave(t, verbatimSource)

			if !strings.Contains(got, testCase.want) {
				t.Errorf("SaveAs did not preserve it verbatim.\n  want to find:\n%s\n\n  got:\n%s",
					testCase.want, got)
			}
		})
	}
}

// TestSaveAsPreservesTheHeaderByteForByte states the rule the way it will be enforced: what the styler did
// not come to change is unchanged.
//
// [TestSaveAsKeepsVerbatimCommentsVerbatim] checks that each verbatim comment can still be found. This
// checks the stronger thing for the one block whose corruption was measured -- that the file still BEGINS
// with exactly the bytes it began with, so a reflow, a re-indent or a dropped blank line all fail here.
func TestSaveAsPreservesTheHeaderByteForByte(t *testing.T) {

	const header = "// SPDX-License-Identifier: Apache-2.0\n// Copyright Noble Factor. All rights reserved.\n"

	got := cleanupAndSave(t, verbatimSource)

	if !strings.HasPrefix(got, header) {
		head := got
		if len(head) > len(header)+120 {
			head = head[:len(header)+120]
		}
		t.Errorf("the file no longer begins with the header it began with.\n  want prefix:\n%q\n\n  got:\n%q",
			header, head)
	}
}

// cleanupAndSave runs the styler over `source` and returns what it wrote, which is the pair of operations
// `star lint go-style --fix` performs on every file it touches.
//
// Parameters:
//   - `t`: the test.
//   - `source`: Go source to parse, style and save.
//
// Returns:
//   - `string`: the saved file's contents.
func cleanupAndSave(t *testing.T, source string) string {

	t.Helper()

	sf, err := parseSourceFile(source)
	if err != nil {
		t.Fatalf("parseSourceFile: %v", err)
	}

	sf.schemaReg = doctaxonomy.DefaultRegistry()
	sf.spacing = DefaultSpacingRules()
	sf.width = 120

	path := filepath.Join(t.TempDir(), "verbatim.go")
	sf.filename = path

	sf.Cleanup()

	if err := sf.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	return string(saved)
}
