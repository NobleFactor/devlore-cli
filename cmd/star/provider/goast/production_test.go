// SPDX-License-Identifier: Apache-2.0
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

// TestItemProduction_RequiredMissingEmitsNothing pins that a required element with nothing to say emits
// nothing, leaving the violation visible.
//
// This test asserted the opposite until 2026-09-30: it required exactly one block reading
// `Backup TODO(go-style): add summary`. That placeholder made `comment.present` true, so the compliance
// check stopped reporting the missing doc comment -- 2,057 of them across this repository would have been
// silenced by filler and the tree reported clean (#994). A summary is prose; it cannot be derived from a
// name.
func TestItemProduction_RequiredMissingEmitsNothing(t *testing.T) {
	blocks := []comment.Block{
		makeList("not a paragraph"),
	}

	elem := doctaxonomy.SchemaElement{Name: "summary", Required: "true", Consumes: "Paragraph / Heading"}
	prod, err := NewProduction(elem)
	if err != nil {
		t.Fatalf("NewProduction: %v", err)
	}

	output, next := prod.Execute(blocks, 0, elem, styleContext{name: "Backup"})
	if len(output) != 0 {
		t.Fatalf("expected no blocks, got %d: %v", len(output), output)
	}
	if next != 0 {
		t.Errorf("expected cursor unchanged at 0, got %d", next)
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
		makeParagraph("+devlore:root=true"),
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

// TestListProduction_RequiredMissingEmitsNothing pins that a required slots element with nothing found emits
// nothing rather than a header over placeholder items.
//
// Until 2026-09-30 this asserted two blocks: a `Parameters:` header and a list of
// `<name>: TODO(go-style): add description` items. The compliance check for that section is
// `strings.Contains(text, "Parameters:")`, so the header alone satisfied it and the violation vanished while
// nothing was documented (#994).
//
// The parameter NAMES were real -- taken from the signature through `ctx.paramNames` -- and that is worth
// keeping when it can be kept. What a parameter MEANS is not in its type, so a section with true names and
// no descriptions would still pass a check that looks only for the header. Emitting one therefore waits on
// the check requiring a description per item rather than a header per section, which is devlore-cli#938.
func TestListProduction_RequiredMissingEmitsNothing(t *testing.T) {
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

	// One parameter and two, because the deleted TestListProduction_SingleParamStub covered the one-parameter
	// case separately and its only distinct assertion was the stub list's item count. There is no list now,
	// so the two cases differ in nothing -- which is the thing worth asserting.
	for _, testCase := range []struct {
		name       string
		paramNames []string
	}{
		{name: "one parameter", paramNames: []string{"v"}},
		{name: "two parameters", paramNames: []string{"path", "suffix"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			ctx := styleContext{name: "Backup", paramNames: testCase.paramNames}
			output, next := prod.Execute(blocks, 0, elem, ctx)

			if len(output) != 0 {
				t.Fatalf("expected no blocks, got %d: %v", len(output), output)
			}
			if next != 0 {
				t.Errorf("expected cursor unchanged (nothing emitted, input not consumed), got %d", next)
			}
		})
	}
}

// --- sentence splitting tests ---

func TestItemProduction_SplitSentence(t *testing.T) {
	blocks := []comment.Block{
		makeParagraph("NewAccessor creates a Accessor. The value should be a struct."),
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
	if summaryText != "NewAccessor creates a Accessor." {
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
		makeParagraph("NewAccessor creates a Accessor."),
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
