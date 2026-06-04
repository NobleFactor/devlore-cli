// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"go/doc/comment"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

// Production transforms a slice of comment blocks according to a schema element.
// It consumes blocks starting at cursor, produces output blocks, and returns the
// new cursor position.
type Production interface {
	Execute(blocks []comment.Block, cursor int, elem doctaxonomy.SchemaElement, ctx styleContext) (output []comment.Block, next int)
}

// blockTypeName returns the type name of a comment.Block for matching against Consumes.Types.
func blockTypeName(b comment.Block) string {
	switch b.(type) {
	case *comment.Paragraph:
		return "Paragraph"
	case *comment.Heading:
		return "Heading"
	case *comment.Code:
		return "Code"
	case *comment.List:
		return "List"
	default:
		return ""
	}
}

// itemProduction consumes zero or more blocks of specified types, optionally matching a prefix.
type itemProduction struct {
	consumes Consumes
}

// Execute scans blocks from cursor, consuming those whose type matches the consumes spec.
// If a prefix is specified on the schema element, only the first matching block must have
// that prefix. Returns consumed blocks and the new cursor.
func (p *itemProduction) Execute(blocks []comment.Block, cursor int, elem doctaxonomy.SchemaElement, ctx styleContext) ([]comment.Block, int) {
	var output []comment.Block
	pos := cursor
	count := 0

	for pos < len(blocks) {
		if p.consumes.Max >= 0 && count >= p.consumes.Max {
			break
		}

		b := blocks[pos]
		if !p.consumes.Matches(blockTypeName(b)) {
			break
		}

		// Prefix check: for single-match (Max=1), only check the first block.
		// For multi-match (Max=-1), check every block.
		if elem.Prefix != "" {
			if !blockMatchesPrefix(b, elem.Prefix, ctx) {
				break
			}
		}

		// Sentence splitting: extract first sentence, replace block with remainder.
		if count == 0 && elem.Split == "sentence" {
			text := blockText(b)
			if text != "" {
				summaryText, remainderText := splitSentence(text)
				if remainderText != "" {
					output = append(output, &comment.Paragraph{
						Text: []comment.Text{comment.Plain(summaryText)},
					})
					blocks[pos] = &comment.Paragraph{
						Text: []comment.Text{comment.Plain(remainderText)},
					}
					count++
					continue
				}
			}
		}

		output = append(output, b)
		pos++
		count++
	}

	// If required and nothing matched, emit a stub.
	if count == 0 && elem.Required == "true" {
		stub := makeStubParagraph(ctx.name, elem)
		output = append(output, stub)
	}

	return output, pos
}

// listProduction consumes an optional heading paragraph followed by a list block.
// Slot filling is deferred to Step 8c.
type listProduction struct {
	consumes Consumes
}

// Execute scans for a heading paragraph matching the schema's Header field, followed by a List.
// If condition is specified and not met, skips entirely. Slot filling is a placeholder until Step 8c.
func (p *listProduction) Execute(blocks []comment.Block, cursor int, elem doctaxonomy.SchemaElement, ctx styleContext) ([]comment.Block, int) {
	// Check condition.
	if elem.Condition != "" && !evaluateCondition(elem.Condition, ctx) {
		return nil, cursor
	}

	var output []comment.Block
	pos := cursor

	// Look for heading paragraph.
	if pos < len(blocks) {
		if para, ok := blocks[pos].(*comment.Paragraph); ok {
			if paragraphTextStartsWith(para, elem.Header) {
				output = append(output, para)
				pos++
			}
		}
	}

	// Look for list.
	if pos < len(blocks) {
		if list, ok := blocks[pos].(*comment.List); ok {
			output = append(output, list)
			pos++
		}
	}

	// If we found nothing and the element is required, emit stubs.
	if len(output) == 0 && (elem.Required == "true" || elem.Required == "if_condition") {
		output = append(output, makeHeaderParagraph(elem.Header))
		output = append(output, makeStubList(ctx, elem))
	}

	return output, pos
}

// evaluateCondition checks a named condition against the style context.
func evaluateCondition(cond string, ctx styleContext) bool {
	switch cond {
	case "params":
		return len(ctx.paramNames) > 0
	case "returns":
		return len(ctx.returnTypes) > 0
	case "exported":
		return len(ctx.name) > 0 && ctx.name[0] >= 'A' && ctx.name[0] <= 'Z'
	case "receiver":
		// Would need receiver info in styleContext — not yet available.
		return false
	default:
		return false
	}
}

// blockMatchesPrefix checks if a block's text starts with the given prefix pattern.
// Currently does exact prefix matching. Fuzzy matching deferred to Step 8b.
func blockMatchesPrefix(b comment.Block, prefix string, ctx styleContext) bool {
	para, ok := b.(*comment.Paragraph)
	if !ok {
		return false
	}
	text := paragraphPlainText(para)
	target := expandPrefix(prefix, ctx)
	return len(text) >= len(target) && text[:len(target)] == target
}

// expandPrefix substitutes {name} in a prefix pattern.
func expandPrefix(prefix string, ctx styleContext) string {
	if ctx.name != "" {
		return replaceAll(prefix, "{name}", ctx.name)
	}
	return prefix
}

// replaceAll is strings.ReplaceAll without importing strings (already imported in source_file.go).
func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// paragraphPlainText extracts the plain text from a paragraph, joining all Text elements.
func paragraphPlainText(p *comment.Paragraph) string {
	var result string
	for _, t := range p.Text {
		switch v := t.(type) {
		case comment.Plain:
			result += string(v)
		case comment.Italic:
			result += string(v)
		}
	}
	return result
}

// paragraphTextStartsWith checks if a paragraph's plain text starts with the given string.
func paragraphTextStartsWith(p *comment.Paragraph, prefix string) bool {
	if prefix == "" {
		return false
	}
	text := paragraphPlainText(p)
	return len(text) >= len(prefix) && text[:len(prefix)] == prefix
}

// blockText extracts the plain text from any block that has text content.
func blockText(b comment.Block) string {
	switch v := b.(type) {
	case *comment.Paragraph:
		return paragraphPlainText(v)
	case *comment.Heading:
		return paragraphPlainText(&comment.Paragraph{Text: v.Text})
	default:
		return ""
	}
}

// splitSentence splits text at the first sentence boundary (". " or ".\n").
// Returns the first sentence and the remainder. If there's only one sentence,
// remainder is empty.
func splitSentence(text string) (string, string) {
	for i := 0; i < len(text)-1; i++ {
		if text[i] == '.' && (text[i+1] == ' ' || text[i+1] == '\n') {
			summary := strings.TrimSpace(text[:i+1])
			remainder := strings.TrimSpace(text[i+1:])
			return summary, remainder
		}
	}
	return text, ""
}

// makeStubParagraph creates a TODO stub paragraph for a missing required element.
func makeStubParagraph(name string, elem doctaxonomy.SchemaElement) *comment.Paragraph {
	text := name + " TODO(go-style): add summary"
	return &comment.Paragraph{
		Text: []comment.Text{comment.Plain(text)},
	}
}

// makeHeaderParagraph creates a paragraph containing just a section header (e.g., "Parameters:").
func makeHeaderParagraph(header string) *comment.Paragraph {
	return &comment.Paragraph{
		Text: []comment.Text{comment.Plain(header)},
	}
}

// makeStubList creates a stub list with TODO items for each slot name.
func makeStubList(ctx styleContext, elem doctaxonomy.SchemaElement) *comment.List {
	var names []string
	switch elem.Slots {
	case "params":
		names = ctx.paramNames
	case "returns":
		names = ctx.returnTypes
	}

	list := &comment.List{}
	for _, name := range names {
		item := &comment.ListItem{
			Content: []comment.Block{
				&comment.Paragraph{
					Text: []comment.Text{
						comment.Plain(name + ": TODO(go-style): add description"),
					},
				},
			},
		}
		list.Items = append(list.Items, item)
	}
	return list
}

// NewProduction creates a Production from a schema element's production type and consumes string.
func NewProduction(elem doctaxonomy.SchemaElement) (Production, error) {
	consumesStr := elem.Consumes
	if consumesStr == "" {
		// Default consumes based on legacy type field.
		switch elem.Type {
		case "paragraph":
			consumesStr = "Paragraph / Heading"
		case "block":
			consumesStr = "*(Paragraph / Code / Heading)"
		case "section":
			consumesStr = "List"
		case "directive":
			consumesStr = "*Paragraph"
		default:
			consumesStr = "Paragraph"
		}
	}

	c, err := ParseConsumes(consumesStr)
	if err != nil {
		return nil, err
	}

	prod := elem.Production
	if prod == "" {
		// Default production based on legacy type field.
		switch elem.Type {
		case "section":
			prod = "list"
		default:
			prod = "item"
		}
	}

	switch prod {
	case "item":
		return &itemProduction{consumes: c}, nil
	case "list":
		return &listProduction{consumes: c}, nil
	default:
		return &itemProduction{consumes: c}, nil
	}
}
