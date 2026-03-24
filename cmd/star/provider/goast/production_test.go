// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"go/doc/comment"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

func makeParagraph(text string) *comment.Paragraph {
	return &comment.Paragraph{Text: []comment.Text{comment.Plain(text)}}
}

func makeCode(text string) *comment.Code {
	return &comment.Code{Text: text}
}

func makeHeading(text string) *comment.Heading {
	return &comment.Heading{Text: []comment.Text{comment.Plain(text)}}
}

func makeList(items ...string) *comment.List {
	list := &comment.List{}
	for _, item := range items {
		list.Items = append(list.Items, &comment.ListItem{
			Content: []comment.Block{makeParagraph(item)},
		})
	}
	return list
}

// --- itemProduction tests ---

func TestItemProduction_SingleParagraph(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("Backup creates a backup."),
		makeParagraph("Extended description."),
	}

	elem := doctaxonomy.SchemaElement{Name: "summary", Consumes: "Paragraph / Heading"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "Backup"})
	if len(output) != 1 {
		t.Fatalf("expected 1 output block, got %d", len(output))
	}
	if next != 1 {
		t.Errorf("expected cursor at 1, got %d", next)
	}
}

func TestItemProduction_ZeroOrMore(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("First paragraph."),
		makeCode("code block"),
		makeParagraph("Second paragraph."),
		makeList("not consumed"),
	}

	elem := doctaxonomy.SchemaElement{Name: "description", Consumes: "*(Paragraph / Code)"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{})
	if len(output) != 3 {
		t.Fatalf("expected 3 output blocks, got %d", len(output))
	}
	if next != 3 {
		t.Errorf("expected cursor at 3, got %d", next)
	}
}

func TestItemProduction_ZeroOrMore_Empty(t *testing.T) {
	blocks := []comment.Block{
		makeList("not a paragraph"),
	}

	elem := doctaxonomy.SchemaElement{Name: "description", Consumes: "*(Paragraph / Code)"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{})
	if len(output) != 0 {
		t.Fatalf("expected 0 output blocks, got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor at 0, got %d", next)
	}
}

func TestItemProduction_RequiredMissing(t *testing.T) {
	blocks := []comment.Block{
		makeList("not a paragraph"),
	}

	elem := doctaxonomy.SchemaElement{Name: "summary", Required: "true", Consumes: "Paragraph / Heading"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "Backup"})
	if len(output) != 1 {
		t.Fatalf("expected 1 stub block, got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged at 0, got %d", next)
	}
	text := paragraphPlainText(output[0].(*comment.Paragraph))
	if text != "Backup TODO(go-style): add summary" {
		t.Errorf("unexpected stub: %q", text)
	}
}

func TestItemProduction_PrefixMatch(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("Backup creates a copy."),
		makeParagraph("Other text."),
	}

	elem := doctaxonomy.SchemaElement{Name: "summary", Prefix: "{name}", Consumes: "Paragraph / Heading"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "Backup"})
	if len(output) != 1 {
		t.Fatalf("expected 1 block, got %d", len(output))
	}
	if next != 1 {
		t.Errorf("expected cursor at 1, got %d", next)
	}
}

func TestItemProduction_PrefixNoMatch(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("creates a copy."),
	}

	elem := doctaxonomy.SchemaElement{Name: "summary", Prefix: "{name}", Consumes: "Paragraph / Heading"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "Backup"})
	if len(output) != 0 {
		t.Fatalf("expected 0 blocks (prefix mismatch), got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged, got %d", next)
	}
}

func TestItemProduction_DirectivePrefix(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("+devlore:defaults overwrite=true"),
		makeParagraph("+devlore:access=both"),
		makeParagraph("Not a directive."),
	}

	elem := doctaxonomy.SchemaElement{Name: "directives", Prefix: "+", Consumes: "*Paragraph"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{})
	if len(output) != 2 {
		t.Fatalf("expected 2 directive blocks, got %d", len(output))
	}
	if next != 2 {
		t.Errorf("expected cursor at 2, got %d", next)
	}
}

// --- listProduction tests ---

func TestListProduction_HeadingAndList(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("Parameters:"),
		makeList("path: the file path", "name: the name"),
	}

	elem := doctaxonomy.SchemaElement{
		Name:       "parameters",
		Production: "list",
		Header:     "Parameters:",
		Condition:  "params",
		Consumes:   "List",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	ctx := styleContext{name: "Backup", paramNames: []string{"path", "name"}}
	output, next := prod.Execute(blocks, 0, elem, ctx)
	if len(output) != 2 {
		t.Fatalf("expected 2 blocks (heading + list), got %d", len(output))
	}
	if next != 2 {
		t.Errorf("expected cursor at 2, got %d", next)
	}
}

func TestListProduction_ConditionFalse(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("Parameters:"),
		makeList("path: the file path"),
	}

	elem := doctaxonomy.SchemaElement{
		Name:       "parameters",
		Production: "list",
		Header:     "Parameters:",
		Condition:  "params",
		Consumes:   "List",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	ctx := styleContext{name: "Backup"} // no params
	output, next := prod.Execute(blocks, 0, elem, ctx)
	if len(output) != 0 {
		t.Fatalf("expected 0 blocks (condition false), got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged, got %d", next)
	}
}

func TestListProduction_RequiredStub(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("Some other text."),
	}

	elem := doctaxonomy.SchemaElement{
		Name:       "parameters",
		Production: "list",
		Header:     "Parameters:",
		Condition:  "params",
		Required:   "if_condition",
		Slots:      "params",
		Consumes:   "List",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	ctx := styleContext{name: "Backup", paramNames: []string{"path", "suffix"}}
	output, next := prod.Execute(blocks, 0, elem, ctx)
	if len(output) != 2 {
		t.Fatalf("expected 2 stub blocks (heading + list), got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged (stubs inserted, input not consumed), got %d", next)
	}

	// Verify stub list has items for each param.
	list, ok := output[1].(*comment.List)
	if !ok {
		t.Fatal("expected List as second output block")
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 list items, got %d", len(list.Items))
	}
}

// --- sentence splitting tests ---

func TestItemProduction_SplitSentence(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("NewAccessor creates a ConfigAccessor. The value should be a struct."),
		makeList("not consumed"),
	}

	elem := doctaxonomy.SchemaElement{
		Name:     "summary",
		Consumes: "Paragraph / Heading",
		Prefix:   "{name}",
		Split:    "sentence",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "NewAccessor"})
	if len(output) != 1 {
		t.Fatalf("expected 1 output block (summary), got %d", len(output))
	}

	summaryText := paragraphPlainText(output[0].(*comment.Paragraph))
	if summaryText != "NewAccessor creates a ConfigAccessor." {
		t.Errorf("summary = %q, want first sentence only", summaryText)
	}

	// The remainder should replace the original block for body to consume.
	if next != 0 {
		t.Errorf("expected cursor at 0 (remainder replaces block), got %d", next)
	}
	remainderText := paragraphPlainText(blocks[0].(*comment.Paragraph))
	if remainderText != "The value should be a struct." {
		t.Errorf("remainder = %q, want second sentence", remainderText)
	}
}

func TestItemProduction_SplitSentence_SingleSentence(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("NewAccessor creates a ConfigAccessor."),
	}

	elem := doctaxonomy.SchemaElement{
		Name:     "summary",
		Consumes: "Paragraph / Heading",
		Prefix:   "{name}",
		Split:    "sentence",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "NewAccessor"})
	if len(output) != 1 {
		t.Fatalf("expected 1 output block, got %d", len(output))
	}
	if next != 1 {
		t.Errorf("expected cursor at 1 (no remainder), got %d", next)
	}
}

func TestListProduction_SingleParamStub(t *testing.T) {
	// No heading, no list — just a paragraph that doesn't match the header.
	blocks := []comment.Block{
		makeParagraph("Unrelated text."),
	}

	elem := doctaxonomy.SchemaElement{
		Name:       "parameters",
		Production: "list",
		Header:     "Parameters:",
		Condition:  "params",
		Required:   "if_condition",
		Slots:      "params",
		Consumes:   "List",
	}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	ctx := styleContext{name: "NewAccessor", paramNames: []string{"v"}}
	output, next := prod.Execute(blocks, 0, elem, ctx)
	if len(output) != 2 {
		t.Fatalf("expected 2 stub blocks (heading + list), got %d", len(output))
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged (stubs inserted, input not consumed), got %d", next)
	}

	// Verify stub list has one item.
	list, ok := output[1].(*comment.List)
	if !ok {
		t.Fatal("expected List as second output block")
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 list item, got %d", len(list.Items))
	}
}

// --- NewProduction from legacy type field ---

func TestNewProduction_LegacyParagraph(t *testing.T) {
	elem := doctaxonomy.SchemaElement{Type: "paragraph"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	if _, ok := prod.(*itemProduction); !ok {
		t.Error("expected itemProduction for paragraph type")
	}
}

func TestNewProduction_LegacySection(t *testing.T) {
	elem := doctaxonomy.SchemaElement{Type: "section"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}
	if _, ok := prod.(*listProduction); !ok {
		t.Error("expected listProduction for section type")
	}
}
