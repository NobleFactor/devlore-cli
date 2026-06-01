// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"fmt"
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

// CommentStyle identifies how a comment is classified for formatting.
type CommentStyle int

const (
	// StyleCopyright is an SPDX + Copyright header. Verbatim.
	StyleCopyright CommentStyle = iota
	// StyleDelineator is a separator line (3+ repeated =, -, ~, *). Verbatim.
	StyleDelineator
	// StyleRegionMarker is a region/endregion marker. Verbatim.
	StyleRegionMarker
	// StyleSectionHeader is a short label like "// Fallible actions". Verbatim.
	StyleSectionHeader
	// StylePackageDoc is a package-level doc comment. Taxonomy pipeline, summary/body split.
	StylePackageDoc
	// StyleImportDoc is an import declaration doc comment.
	StyleImportDoc
	// StyleProse is a multi-line floating comment. go/doc/comment fill and wrap.
	StyleProse
	// StyleFuncDoc is a function/method doc comment. Taxonomy pipeline.
	StyleFuncDoc
	// StyleGenDeclDoc is a type/var/const doc comment. Taxonomy pipeline.
	StyleGenDeclDoc
)

// Decl is any top-level declaration in source order.
type Decl interface {
	DeclName() string
	DeclKind() string
	DeclComment() DocComment
	DeclStyle() CommentStyle
}

// DocComment is the mutable doc comment attached to a declaration.
type DocComment struct {
	doc     *comment.Doc
	present bool
	style   CommentStyle
}

// Text returns the comment text without // prefix, or nil if no comment is present.
func (dc DocComment) Text() any {
	if !dc.present || dc.doc == nil {
		return nil
	}
	return docToText(dc.doc)
}

// Style returns the comment style.
func (dc DocComment) Style() CommentStyle { return dc.style }

// docToText renders a *comment.Doc to plain text without // prefix.
func docToText(doc *comment.Doc) string {
	if doc == nil {
		return ""
	}
	var pr comment.Printer
	return strings.TrimRight(string(pr.Text(doc)), "\n")
}

// textToDoc parses plain text (without // prefix) into a *comment.Doc.
func textToDoc(text string) *comment.Doc {
	var p comment.Parser
	return p.Parse(text)
}

// SpacingRules controls blank lines between declarations. Named settings following the JetBrains IDEA model. Each
// value is the number of blank lines to insert in that context.
type SpacingRules struct {
	AfterPackage        int `yaml:"after_package"`
	AfterImports        int `yaml:"after_imports"`
	BetweenFunctions    int `yaml:"between_functions"`
	BetweenMethods      int `yaml:"between_methods"`
	BeforeTypeMethods   int `yaml:"before_type_methods"`
	AroundRegionMarkers int `yaml:"around_region_markers"`
	AroundDelineators   int `yaml:"around_delineators"`
}

// DefaultSpacingRules returns spacing rules with all settings at 1 blank line.
func DefaultSpacingRules() SpacingRules {
	return SpacingRules{
		AfterPackage:        1,
		AfterImports:        1,
		BetweenFunctions:    1,
		BetweenMethods:      1,
		BeforeTypeMethods:   1,
		AroundRegionMarkers: 1,
		AroundDelineators:   1,
	}
}

// =============================================================================
// TREE TYPES
// =============================================================================

// SourceFile is the semantic tree for a single Go source file.
//
// Mirrors the go/ast field-struct shape: parsed data is exposed on exported fields (which the starlark bridge
// projects as read-only properties), while methods are reserved for actions (Cleanup/Save) and parameterized
// lookups (GetType/GetFunc). The exported fields are precomputed at LoadSourceFile and immutable thereafter.
type SourceFile struct {
	PackageName string         `starlark:"package_name"` // package name
	Types       []*GenDeclNode `starlark:"types"`        // type declarations
	Vars        []*GenDeclNode `starlark:"vars"`         // var declarations
	Consts      []*GenDeclNode `starlark:"consts"`       // const declarations
	Funcs       []*FuncDecl    `starlark:"funcs"`        // top-level functions (no receiver)
	Decls       []Decl         `starlark:"decls"`        // all declarations in source order

	source    string
	filename  string
	fset      *token.FileSet
	file      *ast.File
	genDecls  []*GenDeclNode
	typeIndex map[string]*GenDeclNode
	funcIndex map[string]*FuncDecl

	// Provider-derived state stamped at LoadSourceFile time. Replaces the prior pattern of stashing these on
	// RuntimeEnvironment.variables — goast owns its own state, the runtime environment doesn't.
	schemaReg *doctaxonomy.SchemaRegistry
	spacing   SpacingRules
	width     int
}

// GenDeclNode represents a general declaration (type, var, const, import).
//
// Wraps *ast.GenDecl. One entry per GenDecl in the tree, regardless of how many specs it contains.
type GenDeclNode struct {
	Name    string             `starlark:"name"`    // declared name (first spec)
	Methods []*FuncDecl        `starlark:"methods"` // methods on this type (TYPE decls)
	Entries []ConstEntryDetail `starlark:"entries"` // const entries (CONST decls)

	genDecl     *ast.GenDecl
	comment     DocComment
	code        string
	methodIndex map[string]*FuncDecl
}

// FuncDecl represents a function or method declaration.
type FuncDecl struct {
	Name    string        `starlark:"name"`    // function/method name
	Params  []ParamDetail `starlark:"params"`  // parameters
	Returns string        `starlark:"returns"` // return type string

	node    *ast.FuncDecl
	comment DocComment
	code    string
}

// CommentDecl represents a floating comment not attached to any declaration.
type CommentDecl struct {
	cg    *ast.CommentGroup
	doc   *comment.Doc
	style CommentStyle
}

// =============================================================================
// Decl interface — GenDeclNode
// =============================================================================

// DeclName returns the name of the first spec (type name, var name, const name, or "import").
func (gd *GenDeclNode) DeclName() string { return genDeclName(gd.genDecl) }

// DeclKind returns "type", "var", "const", or "import".
func (gd *GenDeclNode) DeclKind() string { return strings.ToLower(gd.genDecl.Tok.String()) }

// DeclComment returns the doc comment.
func (gd *GenDeclNode) DeclComment() DocComment { return gd.comment }

// DeclStyle returns the comment style.
func (gd *GenDeclNode) DeclStyle() CommentStyle { return gd.comment.style }

// =============================================================================
// Decl interface — FuncDecl
// =============================================================================

// DeclName returns the function or method name.
func (fd *FuncDecl) DeclName() string { return fd.node.Name.Name }

// DeclKind returns "func" or "method".
func (fd *FuncDecl) DeclKind() string {
	if fd.node.Recv != nil {
		return "method"
	}
	return "func"
}

// DeclComment returns the doc comment.
func (fd *FuncDecl) DeclComment() DocComment { return fd.comment }

// DeclStyle returns the comment style.
func (fd *FuncDecl) DeclStyle() CommentStyle { return fd.comment.style }

// =============================================================================
// Decl interface — CommentDecl
// =============================================================================

// DeclName returns empty — floating comments have no name.
func (cd *CommentDecl) DeclName() string { return "" }

// DeclKind returns "comment".
func (cd *CommentDecl) DeclKind() string { return "comment" }

// DeclComment returns nil — the comment IS the declaration, not attached to one.
func (cd *CommentDecl) DeclComment() DocComment { return DocComment{} }

// DeclStyle returns the comment style.
func (cd *CommentDecl) DeclStyle() CommentStyle { return cd.style }

// =============================================================================
// CommentDecl query methods
// =============================================================================

// Text returns the comment text without // prefix.
func (cd *CommentDecl) Text() string { return docToText(cd.doc) }

// Style returns the comment style.
func (cd *CommentDecl) Style() CommentStyle { return cd.style }

// =============================================================================
// SourceFile query methods
// =============================================================================

// GetType returns a type GenDecl by name, or nil if not found.
func (sf *SourceFile) GetType(name string) *GenDeclNode { return sf.typeIndex[name] }

// GetFunc returns a function declaration by name, or nil if not found.
func (sf *SourceFile) GetFunc(name string) *FuncDecl { return sf.funcIndex[name] }

// Name returns the filename.
func (sf *SourceFile) Name() string { return sf.filename }

// genDeclsByTok filters GenDecl nodes to those of the given token kind. Used at LoadSourceFile to precompute the
// Types / Vars / Consts fields.
func genDeclsByTok(genDecls []*GenDeclNode, tok token.Token) []*GenDeclNode {
	var result []*GenDeclNode
	for _, gd := range genDecls {
		if gd.genDecl.Tok == tok {
			result = append(result, gd)
		}
	}
	return result
}

// schemaRegistry returns the schema registry stamped on this file at LoadSourceFile time. Falls back to the
// default registry if no value was stamped.
func (sf *SourceFile) schemaRegistry() *doctaxonomy.SchemaRegistry {
	if sf.schemaReg != nil {
		return sf.schemaReg
	}
	return doctaxonomy.DefaultRegistry()
}

// spacingRules returns the spacing rules stamped on this file at LoadSourceFile time. Falls back to
// [DefaultSpacingRules] if no rules were stamped.
func (sf *SourceFile) spacingRules() SpacingRules {
	if sf.spacing != (SpacingRules{}) {
		return sf.spacing
	}
	return DefaultSpacingRules()
}

// lineWidth returns the line-width budget stamped on this file at LoadSourceFile time. Defaults to 120 when
// zero.
func (sf *SourceFile) lineWidth() int {
	if sf.width > 0 {
		return sf.width
	}
	return 120
}

// =============================================================================
// GenDeclNode query methods
// =============================================================================

// Comment returns the doc comment.
func (gd *GenDeclNode) Comment() DocComment { return gd.comment }

// Kind returns the token type (token.TYPE, token.VAR, token.CONST, token.IMPORT).
func (gd *GenDeclNode) Kind() token.Token { return gd.genDecl.Tok }

// Specs returns the underlying ast.Spec slice.
func (gd *GenDeclNode) Specs() []ast.Spec { return gd.genDecl.Specs }

// GetMethod returns a method by name, or nil if not found.
func (gd *GenDeclNode) GetMethod(name string) *FuncDecl {
	if gd.methodIndex == nil {
		return nil
	}
	return gd.methodIndex[name]
}

// constEntries extracts const entry details (name + value) for a CONST GenDecl, or nil otherwise. Used at
// LoadSourceFile to precompute the GenDeclNode.Entries field.
func constEntries(genDecl *ast.GenDecl) []ConstEntryDetail {
	if genDecl.Tok != token.CONST {
		return nil
	}
	var result []ConstEntryDetail
	for _, spec := range genDecl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		entry := ConstEntryDetail{Name: vs.Names[0].Name}
		if len(vs.Values) > 0 {
			if lit, ok := vs.Values[0].(*ast.BasicLit); ok {
				entry.Value = strings.Trim(lit.Value, `"`)
			}
		}
		result = append(result, entry)
	}
	return result
}

// ConstEntryDetail holds name and value for a single constant in a group.
//
// Mirrors the go/ast field-struct shape: data is read directly off exported fields, which the starlark bridge
// projects as read-only properties (no getters, no codegen).
type ConstEntryDetail struct {
	Name  string `starlark:"name"`
	Value string `starlark:"value"`
}

// =============================================================================
// FuncDecl query methods
// =============================================================================

// Comment returns the doc comment.
func (fd *FuncDecl) Comment() DocComment { return fd.comment }

// ReceiverType returns the receiver type name, or empty for top-level functions.
func (fd *FuncDecl) ReceiverType() string {
	if fd.node.Recv == nil || len(fd.node.Recv.List) == 0 {
		return ""
	}
	return receiverTypeName(fd.node.Recv.List[0].Type)
}

// =============================================================================
// SourceFile operations
// =============================================================================

// styleContext carries the data that distinguishes one styler call from another.
type styleContext struct {
	nodeType    string
	name        string
	paramNames  []string
	returnTypes []string
}

// styleDoc is the single styler. Takes a DocComment and style data, returns a new DocComment.
// Iterates schema elements in order, executes productions, assembles the output block list.
func (sf *SourceFile) styleDoc(dc DocComment, ctx styleContext) DocComment {
	schema := sf.schemaRegistry().Lookup(ctx.nodeType, "go")
	if schema == nil {
		return dc
	}

	// Get input blocks. Empty doc for absent comments.
	var blocks []comment.Block
	if dc.present && dc.doc != nil {
		blocks = dc.doc.Content
	}

	// Sort schema elements by order.
	elems := make([]doctaxonomy.SchemaElement, len(schema.Elements))
	copy(elems, schema.Elements)
	sort.Slice(elems, func(i, j int) bool {
		return elems[i].Order < elems[j].Order
	})

	// Propagate SummaryPrefix from schema to the summary element if it has no prefix.
	for i := range elems {
		if elems[i].Name == "summary" && elems[i].Prefix == "" && schema.SummaryPrefix != "" {
			elems[i].Prefix = strings.ReplaceAll(schema.SummaryPrefix, `\b`, "")
		}
	}

	// Execute productions in schema order.
	var output []comment.Block
	cursor := 0
	for _, elem := range elems {
		prod, err := NewProduction(elem)
		if err != nil {
			continue
		}
		out, next := prod.Execute(blocks, cursor, elem, ctx)
		output = append(output, out...)
		cursor = next
	}

	// Append unclaimed blocks at the end.
	if cursor < len(blocks) {
		output = append(output, blocks[cursor:]...)
	}

	return DocComment{
		doc:     &comment.Doc{Content: output},
		present: true,
		style:   dc.style,
	}
}

// Cleanup dispatches the single styler for each declaration based on its node type.
func (sf *SourceFile) Cleanup() {
	for _, decl := range sf.Decls {
		switch decl.DeclStyle() {
		case StyleFuncDoc:
			fd, ok := decl.(*FuncDecl)
			if !ok {
				continue
			}
			fd.comment = sf.styleDoc(fd.comment, styleContext{
				nodeType:    "FuncDecl",
				name:        fd.node.Name.Name,
				paramNames:  astParamNames(fd.node.Type.Params),
				returnTypes: astReturnTypes(fd.node.Type.Results),
			})

		case StyleGenDeclDoc:
			gd, ok := decl.(*GenDeclNode)
			if !ok {
				continue
			}
			gd.comment = sf.styleDoc(gd.comment, styleContext{
				nodeType: genDeclNodeType(gd.genDecl.Tok),
				name:     genDeclName(gd.genDecl),
			})

		case StylePackageDoc:
			if cd, ok := decl.(*CommentDecl); ok {
				dc := sf.styleDoc(
					DocComment{doc: cd.doc, present: true, style: StylePackageDoc},
					styleContext{nodeType: "PkgPath", name: sf.file.Name.Name},
				)
				cd.doc = dc.doc
			}

		case StyleImportDoc, StyleCopyright, StyleDelineator, StyleRegionMarker, StyleSectionHeader, StyleProse:
			// No styling.
		}
	}
}

// genDeclNodeType returns the schema node type for a GenDecl token.
func genDeclNodeType(tok token.Token) string {
	switch tok {
	case token.TYPE:
		return "GenDecl"
	case token.VAR:
		return "GenDecl"
	case token.CONST:
		return "GenDecl"
	case token.IMPORT:
		return "GenDecl"
	default:
		return "GenDecl"
	}
}

// Save serializes the tree to the original file.
func (sf *SourceFile) Save() error {
	return sf.SaveAs(sf.filename)
}

// SaveAs serializes the tree to the specified path.
func (sf *SourceFile) SaveAs(path string) error {
	var b strings.Builder
	width := sf.lineWidth()

	packageEmitted := false
	hasPackageDoc := false
	prevKind := ""
	for _, decl := range sf.Decls {
		kind := decl.DeclKind()

		// Preamble: copyright and package doc come before package clause.
		if !packageEmitted {
			if cd, ok := decl.(*CommentDecl); ok {
				if cd.style == StyleCopyright || cd.style == StylePackageDoc {
					if prevKind != "" {
						b.WriteString("\n\n")
					}
					b.WriteString(renderDoc(cd.doc, width))
					if cd.style == StylePackageDoc {
						hasPackageDoc = true
					}
					prevKind = kind
					continue
				}
			}
			// Package doc comment must be directly above the package keyword
			// (no blank line). Copyright gets a blank line separator.
			if hasPackageDoc {
				b.WriteString("\n")
			} else if prevKind != "" {
				b.WriteString("\n\n")
			}
			b.WriteString("package ")
			b.WriteString(sf.file.Name.Name)
			packageEmitted = true
			prevKind = "package"
		}

		// Spacing between declarations.
		lines := sf.spacingBetween(prevKind, kind)
		b.WriteString(strings.Repeat("\n", lines+1))

		// Emit the declaration.
		switch d := decl.(type) {
		case *FuncDecl:
			sf.emitDecl(&b, d.comment, d.code, width)
		case *GenDeclNode:
			sf.emitDecl(&b, d.comment, d.code, width)
		case *CommentDecl:
			b.WriteString(renderDoc(d.doc, width))
		}

		prevKind = kind
	}

	if !packageEmitted {
		b.WriteString("package ")
		b.WriteString(sf.file.Name.Name)
	}

	result := b.String()
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return os.WriteFile(path, []byte(result), 0o644)
}

// emitDecl writes a doc comment (rendered through go/doc/comment) followed by code.
func (sf *SourceFile) emitDecl(b *strings.Builder, dc DocComment, code string, width int) {
	if dc.present && dc.doc != nil {
		b.WriteString(renderDoc(dc.doc, width))
		b.WriteString("\n")
	}
	b.WriteString(code)
}

// renderDoc renders a *comment.Doc through go/doc/comment.Printer.
func renderDoc(doc *comment.Doc, width int) string {
	if doc == nil {
		return ""
	}
	var pr comment.Printer
	pr.TextPrefix = "// "
	pr.TextCodePrefix = "//\t"
	pr.TextWidth = width - 3

	out := string(pr.Text(doc))
	return strings.TrimRight(out, "\n")
}

// CheckCompliance reports style violations. No mutation, no I/O.
func (sf *SourceFile) CheckCompliance() []ComplianceViolation {
	var violations []ComplianceViolation

	for _, decl := range sf.Decls {
		switch d := decl.(type) {
		case *FuncDecl:
			if !d.comment.present {
				violations = append(violations, ComplianceViolation{
					Name:    d.Name,
					Kind:    d.DeclKind(),
					Message: d.Name + ": missing doc comment",
				})
				continue
			}
			text := docToText(d.comment.doc)
			if len(d.Params) > 0 && !strings.Contains(text, "Parameters:") {
				violations = append(violations, ComplianceViolation{
					Name:    d.Name,
					Kind:    d.DeclKind(),
					Message: d.Name + ": missing Parameters section",
				})
			}
			if d.Returns != "" && !strings.Contains(text, "Returns:") {
				violations = append(violations, ComplianceViolation{
					Name:    d.Name,
					Kind:    d.DeclKind(),
					Message: d.Name + ": missing Returns section",
				})
			}
		case *GenDeclNode:
			if d.genDecl.Tok == token.TYPE && !d.comment.present {
				violations = append(violations, ComplianceViolation{
					Name:    d.Name,
					Kind:    d.DeclKind(),
					Message: d.Name + ": missing doc comment",
				})
			}
		}
	}

	return violations
}

// ComplianceViolation represents a single style check result.
type ComplianceViolation struct {
	Name    string `starlark:"name"`
	Kind    string `starlark:"kind"`
	Message string `starlark:"message"`
}

// spacingBetween returns the number of blank lines between two adjacent declaration kinds.
func (sf *SourceFile) spacingBetween(above, below string) int {
	s := sf.spacingRules()

	if above == "comment" || below == "comment" {
		return s.AroundRegionMarkers
	}

	switch above {
	case "package":
		return s.AfterPackage
	case "import":
		return s.AfterImports
	case "type":
		if below == "method" {
			return s.BeforeTypeMethods
		}
	}

	switch {
	case above == "func" && below == "func":
		return s.BetweenFunctions
	case above == "method" && below == "method":
		return s.BetweenMethods
	}

	return 1
}

// =============================================================================
// Builder
// =============================================================================

// extractDeclCode extracts the verbatim source text for a declaration.
func extractDeclCode(source string, fset *token.FileSet, decl ast.Node) string {
	start := fset.Position(decl.Pos()).Offset
	end := fset.Position(decl.End()).Offset
	if end > len(source) {
		end = len(source)
	}
	return source[start:end]
}

// LoadSourceFile parses Go source content and builds a semantic tree.
func LoadSourceFile(content string) (*SourceFile, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	sf := &SourceFile{
		source:    content,
		fset:      fset,
		file:      file,
		typeIndex: make(map[string]*GenDeclNode),
		funcIndex: make(map[string]*FuncDecl),
	}

	// Classify comment groups: doc comments are attached to declarations,
	// body comments are inside declaration bodies, everything else is floating.
	docCGs := map[*ast.CommentGroup]bool{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				docCGs[d.Doc] = true
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				docCGs[d.Doc] = true
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Doc != nil {
						docCGs[s.Doc] = true
					}
				case *ast.ValueSpec:
					if s.Doc != nil {
						docCGs[s.Doc] = true
					}
				}
			}
		}
	}

	if file.Doc != nil {
		docCGs[file.Doc] = true
	}

	bodyCGs := map[*ast.CommentGroup]bool{}
	for _, decl := range file.Decls {
		for _, cg := range file.Comments {
			if !docCGs[cg] && cg.Pos() >= decl.Pos() && cg.End() <= decl.End() {
				bodyCGs[cg] = true
			}
		}
	}

	// Collect all positioned items for source-order interleaving.
	type positioned struct {
		pos  token.Pos
		decl Decl
	}
	var items []positioned

	// Package doc as a CommentDecl with StylePackageDoc.
	if file.Doc != nil {
		items = append(items, positioned{
			pos: file.Doc.Pos(),
			decl: &CommentDecl{
				cg:    file.Doc,
				doc:   textToDoc(file.Doc.Text()),
				style: StylePackageDoc,
			},
		})
	}

	type pendingMethod struct {
		typeName string
		decl     *FuncDecl
	}
	var pending []pendingMethod

	// Declarations.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			fd := &FuncDecl{
				Name:    d.Name.Name,
				Params:  extractParams(d.Type.Params, nil),
				Returns: returnTypeString(d.Type.Results),
				node:    d,
				comment: docFromCommentGroup(d.Doc, StyleFuncDoc),
				code:    extractDeclCode(content, fset, d),
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				typeName := strings.TrimPrefix(receiverTypeName(d.Recv.List[0].Type), "*")
				pending = append(pending, pendingMethod{typeName: typeName, decl: fd})
			} else {
				sf.Funcs = append(sf.Funcs, fd)
				sf.funcIndex[d.Name.Name] = fd
			}
			items = append(items, positioned{pos: d.Pos(), decl: fd})

		case *ast.GenDecl:
			style := StyleGenDeclDoc
			if d.Tok == token.IMPORT {
				style = StyleImportDoc
			}
			gd := &GenDeclNode{
				Name:        genDeclName(d),
				Entries:     constEntries(d),
				genDecl:     d,
				comment:     docFromGenDecl(d, style),
				code:        extractDeclCode(content, fset, d),
				methodIndex: make(map[string]*FuncDecl),
			}
			sf.genDecls = append(sf.genDecls, gd)

			// Index type names for method association and GetType lookup.
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						sf.typeIndex[ts.Name.Name] = gd
					}
				}
			}

			items = append(items, positioned{pos: d.Pos(), decl: gd})
		}
	}

	// Floating comments — classify each one.
	for _, cg := range file.Comments {
		if docCGs[cg] || bodyCGs[cg] {
			continue
		}
		rawText := cg.Text()
		style, err := classifyFloatingComment(rawText)
		if err != nil {
			return nil, fmt.Errorf("LoadSourceFile: %w", err)
		}
		items = append(items, positioned{
			pos: cg.Pos(),
			decl: &CommentDecl{
				cg:    cg,
				doc:   textToDoc(rawText),
				style: style,
			},
		})
	}

	// Sort by source position.
	sort.Slice(items, func(i, j int) bool {
		return items[i].pos < items[j].pos
	})

	// Build allDecls in source order.
	for _, item := range items {
		sf.Decls = append(sf.Decls, item.decl)
	}

	// Associate methods with their types.
	for _, pm := range pending {
		if gd, ok := sf.typeIndex[pm.typeName]; ok {
			gd.Methods = append(gd.Methods, pm.decl)
			gd.methodIndex[pm.decl.Name] = pm.decl
		}
	}

	// Precompute the exported view fields (immutable after load) — the bridge projects them as read-only
	// properties. Funcs and Decls were accumulated above; filter the rest from genDecls here.
	sf.PackageName = file.Name.Name
	sf.Types = genDeclsByTok(sf.genDecls, token.TYPE)
	sf.Vars = genDeclsByTok(sf.genDecls, token.VAR)
	sf.Consts = genDeclsByTok(sf.genDecls, token.CONST)

	return sf, nil
}

// =============================================================================
// COMMENT CLASSIFICATION
// =============================================================================

// classifyFloatingComment determines the CommentStyle for a floating comment.
func classifyFloatingComment(text string) (CommentStyle, error) {
	if strings.HasPrefix(text, "SPDX-License-Identifier") {
		return StyleCopyright, nil
	}
	if isDelineatorBlock(text) {
		return StyleDelineator, nil
	}
	if strings.HasPrefix(text, "region ") || text == "endregion" || strings.HasPrefix(text, "endregion ") {
		return StyleRegionMarker, nil
	}
	if !strings.Contains(text, "\n") {
		return StyleSectionHeader, nil
	}
	return StyleProse, nil
}

// =============================================================================
// DOC COMMENT BUILDERS
// =============================================================================

// docFromCommentGroup creates a DocComment from an ast.CommentGroup with a given style.
func docFromCommentGroup(cg *ast.CommentGroup, style CommentStyle) DocComment {
	if cg == nil {
		return DocComment{style: style}
	}
	return DocComment{doc: textToDoc(cg.Text()), present: true, style: style}
}

// docFromGenDecl creates a DocComment for a GenDecl, preferring the single spec's doc when there's only one spec.
func docFromGenDecl(d *ast.GenDecl, style CommentStyle) DocComment {
	if len(d.Specs) == 1 {
		switch s := d.Specs[0].(type) {
		case *ast.TypeSpec:
			if s.Doc != nil {
				return docFromCommentGroup(s.Doc, style)
			}
		case *ast.ValueSpec:
			if s.Doc != nil {
				return docFromCommentGroup(s.Doc, style)
			}
		}
	}
	return docFromCommentGroup(d.Doc, style)
}
