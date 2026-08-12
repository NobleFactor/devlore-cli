// SPDX-License-Identifier: Apache-2.0
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
func (p *itemProduction) Execute(
	blocks []comment.Block, cursor int, elem doctaxonomy.SchemaElement, ctx styleContext,
) (output []comment.Block, next int) {

	pos := cursor
	count := 0

	for pos < len(blocks) {
		b := blocks[pos]
		if !p.canConsume(b, count, elem, ctx) {
			break
		}

		// Sentence splitting: extract first sentence, replace block with remainder.
		if count == 0 && elem.Split == "sentence" {
			if summary, ok := splitFirstSentence(blocks, pos); ok {
				output = append(output, summary)
				count++
				continue
			}
		}

		output = append(output, b)
		pos++
		count++
	}

	// If required and nothing matched, emit a stub.
	if count == 0 && elem.Required == "true" {
		stub := makeStubParagraph(ctx.name)
		output = append(output, stub)
	}

	return output, pos
}

// canConsume reports whether the block at the cursor may join this production: under the Max cap,
// type-matched, and prefix-matched when the element declares a prefix (the first block for
// single-match, every block for multi-match).
//
// Parameters:
//   - `b`: the candidate block.
//   - `count`: the number of blocks already consumed.
//   - `elem`: the schema element driving the production.
//   - `ctx`: the style context for prefix matching.
//
// Returns:
//   - `bool`: true when the block may be consumed.
func (p *itemProduction) canConsume(b comment.Block, count int, elem doctaxonomy.SchemaElement, ctx styleContext) bool {

	if p.consumes.Max >= 0 && count >= p.consumes.Max {
		return false
	}
	if !p.consumes.Matches(blockTypeName(b)) {
		return false
	}

	return elem.Prefix == "" || blockMatchesPrefix(b, elem.Prefix, ctx)
}

// splitFirstSentence splits the block at the cursor at its first sentence boundary: the summary
// paragraph is returned and the block is replaced in place by the remainder.
//
// Parameters:
//   - `blocks`: the block list; `blocks[pos]` is replaced on a successful split.
//   - `pos`: the cursor.
//
// Returns:
//   - `comment.Block`: the first-sentence summary paragraph.
//   - `bool`: false when the block has no text or no remainder to split off.
func splitFirstSentence(blocks []comment.Block, pos int) (comment.Block, bool) {

	text := blockText(blocks[pos])
	if text == "" {
		return nil, false
	}

	summaryText, remainderText := splitSentence(text)
	if remainderText == "" {
		return nil, false
	}

	blocks[pos] = &comment.Paragraph{
		Text: []comment.Text{comment.Plain(remainderText)},
	}

	return &comment.Paragraph{
		Text: []comment.Text{comment.Plain(summaryText)},
	}, true
}

// listProduction consumes an optional heading paragraph followed by a list block.
// Slot filling is deferred to Step 8c.
type listProduction struct {
	consumes Consumes
}

// Execute scans for a heading paragraph matching the schema's Header field, followed by a List.
// If condition is specified and not met, skips entirely. Slot filling is a placeholder until Step 8c.
func (p *listProduction) Execute(
	blocks []comment.Block, cursor int, elem doctaxonomy.SchemaElement, ctx styleContext,
) (output []comment.Block, next int) {
	// Check condition.
	if elem.Condition != "" && !evaluateCondition(elem.Condition, ctx) {
		return nil, cursor
	}

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
		output = append(output, makeHeaderParagraph(elem.Header), makeStubList(ctx, elem))
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
		return ctx.name != "" && ctx.name[0] >= 'A' && ctx.name[0] <= 'Z'
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
func replaceAll(s, old, replacement string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + replacement + s[i+len(old):]
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
func splitSentence(text string) (first, remainder string) {
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
func makeStubParagraph(name string) *comment.Paragraph {
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
