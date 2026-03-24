// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"go/doc/comment"
	"strings"
	"testing"
)

// TestPrinterListFormat verifies how go/doc/comment.Printer formats list items
// so we can decide whether to use its output directly or render lists ourselves.
func TestPrinterListFormat(t *testing.T) {

	// Our current style: "Parameters:" paragraph followed by list items.
	input := `Backup creates a timestamped copy of the resource.

Parameters:
  - resource: The file to back up.
  - opts: Backup options (default: nil).

Returns:
  - Resource: The backup copy.
  - Tombstone: Compensation state for undo.
  - error: Non-nil if the backup failed.`

	var p comment.Parser
	doc := p.Parse(input)

	t.Run("parsed structure", func(t *testing.T) {
		t.Logf("Blocks: %d", len(doc.Content))
		for i, block := range doc.Content {
			switch b := block.(type) {
			case *comment.Paragraph:
				text := renderInlineText(b.Text)
				t.Logf("  [%d] Paragraph: %q", i, text)
			case *comment.List:
				t.Logf("  [%d] List (%d items):", i, len(b.Items))
				for j, item := range b.Items {
					text := ""
					for _, blk := range item.Content {
						if para, ok := blk.(*comment.Paragraph); ok {
							text = renderInlineText(para.Text)
						}
					}
					t.Logf("    [%d.%d] Number=%q Content=%q", i, j, item.Number, text)
				}
			case *comment.Heading:
				text := renderInlineText(b.Text)
				t.Logf("  [%d] Heading: %q", i, text)
			case *comment.Code:
				t.Logf("  [%d] Code: %q", i, b.Text)
			}
		}
	})

	t.Run("Comment output at width 120", func(t *testing.T) {
		var pr comment.Printer
		pr.TextWidth = 120
		output := string(pr.Comment(doc))
		t.Logf("Comment output:\n%s", output)

		// Check that output contains our list items.
		if !strings.Contains(output, "resource") {
			t.Error("output missing 'resource' parameter")
		}
		if !strings.Contains(output, "Tombstone") {
			t.Error("output missing 'Tombstone' return")
		}
	})

	t.Run("Text output at width 117", func(t *testing.T) {
		// TextWidth for text output (no "// " prefix — that's added by TextPrefix).
		var pr comment.Printer
		pr.TextWidth = 117
		pr.TextPrefix = "// "
		pr.TextCodePrefix = "// \t"
		output := string(pr.Text(doc))
		t.Logf("Text output:\n%s", output)
	})

	t.Run("round trip fidelity", func(t *testing.T) {
		var pr comment.Printer
		pr.TextWidth = 120
		output := string(pr.Comment(doc))

		// Re-parse the output.
		doc2 := p.Parse(output)

		// Re-print.
		output2 := string(pr.Comment(doc2))

		if output != output2 {
			t.Errorf("round trip changed output:\n--- first ---\n%s\n--- second ---\n%s", output, output2)
		}
	})
}

// TestPrinterLongListItem tests how the Printer wraps long list item descriptions.
func TestPrinterLongListItem(t *testing.T) {

	input := `Summary line.

Parameters:
  - resource: The file resource to back up, which must exist and be accessible by the current process with read permissions.
  - opts: Backup options controlling timestamp format, destination directory, and overwrite behavior (default: nil).`

	var p comment.Parser
	doc := p.Parse(input)

	var pr comment.Printer
	pr.TextWidth = 120

	output := string(pr.Comment(doc))
	t.Logf("Long list item output:\n%s", output)

	// Document: Printer does NOT wrap list items — they exceed TextWidth.
	// This is known behavior (Q2 finding). We render lists ourselves.
	for i, line := range strings.Split(output, "\n") {
		if len(line) > 120 {
			t.Logf("line %d exceeds 120 columns (%d) — expected (Printer doesn't wrap list items): %q", i+1, len(line), line)
		}
	}
}

// TestPrinterDirectiveHandling tests what happens to +devlore: directives.
func TestPrinterDirectiveHandling(t *testing.T) {

	input := `Summary line.

+devlore:defaults overwrite=true

Parameters:
  - resource: The file to back up.`

	var p comment.Parser
	doc := p.Parse(input)

	t.Run("parsed blocks", func(t *testing.T) {
		for i, block := range doc.Content {
			switch b := block.(type) {
			case *comment.Paragraph:
				t.Logf("  [%d] Paragraph: %q", i, renderInlineText(b.Text))
			case *comment.List:
				t.Logf("  [%d] List (%d items)", i, len(b.Items))
			case *comment.Code:
				t.Logf("  [%d] Code: %q", i, b.Text)
			}
		}
	})

	var pr comment.Printer
	pr.TextWidth = 120
	output := string(pr.Comment(doc))
	t.Logf("Directive handling output:\n%s", output)

	// Check if the directive survived or was reflowed.
	if strings.Contains(output, "+devlore:defaults") {
		t.Log("Directive preserved in output")
	} else {
		t.Log("Directive was DESTROYED or reflowed")
	}
}

// TestPrinterSectionHeaderSurvival tests whether "Parameters:" as a standalone paragraph survives round-trip.
func TestPrinterSectionHeaderSurvival(t *testing.T) {

	input := `Summary line.

Parameters:
  - name: description.`

	var p comment.Parser
	doc := p.Parse(input)

	var pr comment.Printer
	pr.TextWidth = 120
	output := string(pr.Comment(doc))
	t.Logf("Section header output:\n%s", output)

	if !strings.Contains(output, "Parameters:") {
		t.Error("Parameters: header was lost")
	}
}

// renderInlineText concatenates inline text elements for logging.
func renderInlineText(text []comment.Text) string {
	var parts []string
	for _, t := range text {
		switch v := t.(type) {
		case comment.Plain:
			parts = append(parts, string(v))
		case comment.Italic:
			parts = append(parts, string(v))
		case *comment.Link:
			for _, lt := range v.Text {
				if p, ok := lt.(comment.Plain); ok {
					parts = append(parts, string(p))
				}
			}
		case *comment.DocLink:
			for _, lt := range v.Text {
				if p, ok := lt.(comment.Plain); ok {
					parts = append(parts, string(p))
				}
			}
		}
	}
	return strings.Join(parts, "")
}
