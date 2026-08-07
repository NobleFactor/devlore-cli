// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package goast provides Go AST operations as a Starlark receiver.
//
// +devlore:access=immediate
package goast

import (
	"bytes"
	"fmt"

	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	tmpl "text/template"

	cfg "github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides Go AST operations as a Starlark receiver.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	fileCache sync.Map // path → *parsedFile (AST cache)
}

// NewProvider creates a new Provider. Validates that all six comment styles have handlers in the merged
// config. Missing styles are repaired from defaults with a warning. Declares interest in the "config"
// variable so the resolver populates it from the [application.Application]'s source maps at construction
// time.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	assert.NoError("register config parameter", ctx.RegisterParameter(op.Parameter{
		Name: "config",
		Type: reflect.TypeOf((*cfg.Config)(nil)),
	}))
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	return p
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Callable introspects a named function type declaration and returns its parameter list, return type, and doc comment
// (including directives).
func (p *Provider) Callable(path, name string) (CallableResult, error) {
	sources, err := p.parsedSources(path)
	if err != nil {
		return CallableResult{}, fmt.Errorf("goast.callable: %w", err)
	}

	for src := range sources {
		if result, found := callableInFile(src.node, name); found {
			return result, nil
		}
	}

	return CallableResult{}, fmt.Errorf("goast.callable: function type %q not found in %s", name, path)
}

// Calls returns function/method calls within a scope.
//
// +devlore:defaults name=
func (p *Provider) Calls(scope, name string) ([]CallResult, error) {
	fileSet, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.calls: %w", err)
	}

	result := []CallResult{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if cr, matched := callResultFromExpr(fileSet, call, name); matched {
			result = append(result, cr)
		}

		return true
	})

	return result, nil
}

// CheckLineWidth checks content for line-width violations.
//
// Reports over-long lines and under-filled comment lines (where the next word would fit on the current line without
// exceeding width).
func (p *Provider) CheckLineWidth(content string, width int) ([]LineViolation, error) {
	return checkLineWidth(content, width), nil
}

// Composites returns composite literals within a scope.
//
// +devlore:defaults typeName=
func (p *Provider) Composites(scope, typeName string) ([]CompositeResult, error) {
	fileSet, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.composites: %w", err)
	}

	result := []CompositeResult{}
	ast.Inspect(body, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		tn := ""
		if comp.Type != nil {
			tn = typeToString(comp.Type)
		}

		if typeName != "" && tn != typeName {
			return true
		}

		result = append(result, CompositeResult{
			TypeName: tn,
			Line:     fileSet.Position(comp.Pos()).Line,
			Fields:   compositeFields(comp.Elts),
		})

		return true
	})

	return result, nil
}

// ConstGroups returns typed const groups from Go source files.
//
// +devlore:defaults typeName=
func (p *Provider) ConstGroups(path, typeName string) ([]ConstGroupResult, error) {
	sources, err := p.parsedSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.const_groups: %w", err)
	}

	var groups []constGroup
	for src := range sources {
		ast.Inspect(src.node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				return true
			}
			groups = append(groups, constGroupsFromDecl(src.fset, genDecl, filepath.Base(src.path), typeName)...)
			return true
		})
	}

	return constGroupResults(groups), nil
}

// Deps analyzes import dependencies for Go source files at the given path.
func (p *Provider) Deps(path string) (DepsResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return DepsResult{}, fmt.Errorf("goast.deps: %w", err)
	}

	modulePath := detectModulePath(path)

	allFiles := []FileDep{}
	allImports := make(map[string]bool)
	allInternal := make(map[string]bool)
	allExternal := make(map[string]bool)
	allStdlib := make(map[string]bool)

	for _, file := range files {
		fd, err := analyzeFileDeps(file, modulePath)
		if err != nil {
			continue
		}

		allFiles = append(allFiles, fd)

		for _, imp := range fd.Imports {
			allImports[imp.Path] = true
		}

		for _, dep := range fd.InternalDeps {
			allInternal[dep] = true
		}

		for _, dep := range fd.ExternalDeps {
			allExternal[dep] = true
		}

		for _, dep := range fd.StdlibDeps {
			allStdlib[dep] = true
		}
	}

	return DepsResult{
		Files:         allFiles,
		ModulePath:    modulePath,
		AllImports:    mapKeys(allImports),
		InternalDeps:  mapKeys(allInternal),
		ExternalDeps:  mapKeys(allExternal),
		StdlibDeps:    mapKeys(allStdlib),
		InternalCount: len(allInternal),
		ExternalCount: len(allExternal),
		StdlibCount:   len(allStdlib),
	}, nil
}

// Format formats Go source code via go/format.
func (p *Provider) Format(code string) (string, error) {
	formatted, err := format.Source([]byte(code))
	if err != nil {
		return "", fmt.Errorf("goast.format: %w", err)
	}

	return string(formatted), nil
}

// Funcs returns function declarations (non-method) from Go source.
//
// The path parameter accepts either a file/directory path or Go source content directly.
//
// +devlore:defaults name=
func (p *Provider) Funcs(path, name string) ([]FuncResult, error) {
	sources, err := resolveGoSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.funcs: %w", err)
	}

	result := []FuncResult{}
	for _, src := range sources {
		fileSet := token.NewFileSet()
		node, err := parser.ParseFile(fileSet, src.name, src.content, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}

			if name != "" && fn.Name.Name != name {
				continue
			}

			returns := returnTypeString(fn.Type.Results)
			rawDoc := ""
			if fn.Doc != nil {
				rawDoc = commentGroupRaw(fn.Doc)
			}

			result = append(result, FuncResult{
				Name:       fn.Name.Name,
				Returns:    returns,
				Params:     extractParams(fn.Type.Params, nil),
				TypeParams: extractTypeParams(fn.Type.TypeParams),
				File:       src.name,
				Line:       fileSet.Position(fn.Pos()).Line,
				Doc:        rawDoc,
			})
		}
	}

	return result, nil
}

// Methods returns method declarations from Go source.
//
// The path parameter accepts either a file/directory path or Go source content directly.
//
// +devlore:defaults name=,receiverType=,returns=
func (p *Provider) Methods(path, name, receiverType, returns string) ([]MethodResult, error) {
	sources, err := resolveGoSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.methods: %w", err)
	}

	result := []MethodResult{}
	for _, src := range sources {
		fileSet := token.NewFileSet()
		node, err := parser.ParseFile(fileSet, src.name, src.content, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Name == nil {
				continue
			}

			typeName := receiverTypeName(fn.Recv.List[0].Type)
			retStr := returnTypeString(fn.Type.Results)
			if !methodMatches(fn.Name.Name, typeName, retStr, name, receiverType, returns) {
				continue
			}

			result = append(result, methodResultFor(src.name, fileSet, fn, typeName, retStr))
		}
	}

	return result, nil
}

// Metrics computes code metrics for Go source files at the given path.
func (p *Provider) Metrics(path string) (MetricsResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return MetricsResult{}, fmt.Errorf("goast.metrics: %w", err)
	}

	allFiles := []FileMetric{}
	var totals FileMetric

	for _, file := range files {
		fm, err := analyzeFileMetrics(file)
		if err != nil {
			continue
		}

		allFiles = append(allFiles, fm)

		totals.LOC += fm.LOC
		totals.SLOC += fm.SLOC
		totals.Comments += fm.Comments
		totals.Blanks += fm.Blanks
		totals.Functions += fm.Functions
		totals.Methods += fm.Methods
		totals.Structs += fm.Structs
		totals.Interfaces += fm.Interfaces
		totals.Types += fm.Types
		totals.Constants += fm.Constants
		totals.Variables += fm.Variables
		totals.Imports += fm.Imports
		totals.TestFunctions += fm.TestFunctions
	}

	return MetricsResult{
		Files:              allFiles,
		FileCount:          len(files),
		TotalLOC:           totals.LOC,
		TotalSLOC:          totals.SLOC,
		TotalComments:      totals.Comments,
		TotalBlanks:        totals.Blanks,
		TotalFunctions:     totals.Functions,
		TotalMethods:       totals.Methods,
		TotalStructs:       totals.Structs,
		TotalInterfaces:    totals.Interfaces,
		TotalTypes:         totals.Types,
		TotalConstants:     totals.Constants,
		TotalVariables:     totals.Variables,
		TotalImports:       totals.Imports,
		TotalTestFunctions: totals.TestFunctions,
	}, nil
}

// RawString extracts the first backtick string literal from a scope.
func (p *Provider) RawString(scope string) (string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return "", fmt.Errorf("goast.raw_string: %w", err)
	}

	var rawStr string
	ast.Inspect(body, func(n ast.Node) bool {
		if rawStr != "" {
			return false
		}

		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		if strings.HasPrefix(lit.Value, "`") {
			rawStr = strings.Trim(lit.Value, "`")
			return false
		}

		return true
	})

	return rawStr, nil
}

// Render executes a Go text/template against data and returns go/format-formatted Go source code.
func (p *Provider) Render(template string, data any) (string, error) {
	t, err := tmpl.New("render").Funcs(renderFuncs).Parse(template)
	if err != nil {
		return "", fmt.Errorf("goast.render: template parse: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("goast.render: template execution: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("goast.render: format error: %w\nraw output:\n%s", err, buf.String())
	}

	return string(formatted), nil
}

// ReturnString extracts the string literal from a return statement in a scope.
func (p *Provider) ReturnString(scope string) (string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return "", fmt.Errorf("goast.return_string: %w", err)
	}

	return extractReturnString(body), nil
}

// ReturnStrings extracts string elements from a []string{...} return statement in a scope.
func (p *Provider) ReturnStrings(scope string) ([]string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.return_strings: %w", err)
	}

	return extractReturnStrings(body), nil
}

// LoadSourceFile reads a Go source file from disk and parses it into a semantic tree organized by declaration kind.
// The returned SourceFile supports iteration, name-based lookup, and style operations (Reformat, Save, CheckStyle).
// Styling config (schemas, spacing rules, line width) is read from context.
//
// Parameters:
//   - path: the file path to read.
//
// Returns:
//   - *SourceFile: the semantic tree.
//   - error: non-nil if the file cannot be read or parsed.
func (p *Provider) LoadSourceFile(path string) (*SourceFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("goast.load_source_file: %w", err)
	}
	sf, err := LoadSourceFile(string(content))
	if err != nil {
		return nil, fmt.Errorf("goast.load_source_file: %w", err)
	}
	sf.filename = path
	sf.schemaReg = p.schemaRegistry()
	sf.spacing = p.spacingRules()
	sf.width = p.configLineWidth()
	return sf, nil
}

// schemaRegistry returns the schema registry the provider operates on: defaults overlaid with any
// project-config-supplied schemas.
//
// Project config can supply zero, some, or all schema types under `lint.go_style.comment_schemas`.
// The resolution rule is "merge by (NodeType, Format) key, config wins":
//
//   - Schemas not present in config keep their default form.
//   - Schemas present in both default and config are replaced by the config form (full replacement
//     of the schema entry — config-side schemas are not deep-merged into default-side schemas;
//     authors who want partial overrides must spell out the full entry).
//   - Schemas present only in config are added.
//
// Returns the embedded defaults unchanged when no `lint.go_style.comment_schemas` config block is
// present at all.
func (p *Provider) schemaRegistry() *doctaxonomy.SchemaRegistry {
	reg := doctaxonomy.DefaultRegistry()
	overlay := p.configSchemas()
	if overlay == nil {
		return reg
	}
	for _, schema := range overlay.All() {
		reg.Register(schema)
	}
	return reg
}

// configSchemas attempts to build a SchemaRegistry from the resolved "config" variable.
func (p *Provider) configSchemas() *doctaxonomy.SchemaRegistry {
	v, ok := p.RuntimeEnvironment().VariableByName("config")
	if !ok || v.Value == nil {
		return nil
	}
	c, ok := v.Value.(*cfg.Config)
	if !ok || c == nil {
		return nil
	}

	schemasVal := c.Navigate("lint.go_style.comment_schemas")
	if schemasVal == nil {
		return nil
	}

	return schemasFromConfig(schemasVal)
}

// spacingRules reads SpacingRules from the resolved "config" variable, falling back to defaults.
func (p *Provider) spacingRules() SpacingRules {
	v, ok := p.RuntimeEnvironment().VariableByName("config")
	if !ok || v.Value == nil {
		return DefaultSpacingRules()
	}
	c, ok := v.Value.(*cfg.Config)
	if !ok || c == nil {
		return DefaultSpacingRules()
	}

	val := c.Navigate("lint.go_style.spacing_rules")
	if val == nil {
		return DefaultSpacingRules()
	}

	return spacingRulesFromConfig(val)
}

// spacingRulesFromConfig extracts SpacingRules from a config value using reflection.
func spacingRulesFromConfig(val interface{}) SpacingRules {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return DefaultSpacingRules()
	}

	rules := DefaultSpacingRules()
	for name, target := range map[string]*int{
		"AfterPackage":        &rules.AfterPackage,
		"AfterImports":        &rules.AfterImports,
		"BetweenFunctions":    &rules.BetweenFunctions,
		"BetweenMethods":      &rules.BetweenMethods,
		"BeforeTypeMethods":   &rules.BeforeTypeMethods,
		"AroundRegionMarkers": &rules.AroundRegionMarkers,
		"AroundDelineators":   &rules.AroundDelineators,
	} {
		if f := rv.FieldByName(name); f.IsValid() && f.CanInt() {
			*target = int(f.Int())
		}
	}
	return rules
}

// configLineWidth reads the line width from the resolved "config" variable, defaulting to 120.
func (p *Provider) configLineWidth() int {
	v, ok := p.RuntimeEnvironment().VariableByName("config")
	if !ok || v.Value == nil {
		return 120
	}
	c, ok := v.Value.(*cfg.Config)
	if !ok || c == nil {
		return 120
	}
	val := c.Navigate("lint.go_style.line_width")
	if val == nil {
		return 120
	}
	rv := reflect.ValueOf(val)
	if rv.CanInt() {
		return int(rv.Int())
	}
	return 120
}

// SortDeclarations reorders function/method declarations within a scope of a Go file.
//
// Preserves doc comments and blank lines attached to each declaration. Returns the modified file content.
func (p *Provider) SortDeclarations(path, scope, order string) (string, error) {

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, path, content, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	startLine, endLine, err := parseScopeRange(scope, len(strings.Split(string(content), "\n")))
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	blocks := declBlocksInScope(fileSet, node, startLine, endLine)
	if len(blocks) <= 1 {
		return string(content), nil
	}

	switch order {
	case "alphabetical", "":
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i].name < blocks[j].name
		})
	default:
		return "", fmt.Errorf("goast.sort_declarations: unknown order: %s", order)
	}

	return spliceSortedBlocks(lines, blocks), nil
}

// Structs returns struct definitions from Go source files.
func (p *Provider) Structs(path string) ([]StructResult, error) {
	sources, err := p.parsedSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.structs: %w", err)
	}

	result := []StructResult{}
	for src := range sources {
		ast.Inspect(src.node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				return true
			}

			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				result = append(result, StructResult{
					Name:   ts.Name.Name,
					File:   filepath.Base(src.path),
					Line:   src.fset.Position(ts.Pos()).Line,
					Fields: structFields(st),
				})
			}

			return true
		})
	}

	return result, nil
}

// TypeDoc returns the doc comment for a named type declaration.
//
// +devlore:defaults name=
func (p *Provider) TypeDoc(path, name string) (string, error) {
	if name == "" {
		name = "Provider"
	}

	sources, err := p.parsedSources(path)
	if err != nil {
		return "", fmt.Errorf("goast.type_doc: %w", err)
	}

	for src := range sources {
		if doc, found := typeDocInFile(src.node, name); found {
			return doc, nil
		}
	}

	return "", nil
}

// endregion

// endregion

// =============================================================================
// UNEXPORTED HELPERS
// =============================================================================

// callableInFile finds the named function-type declaration in one parsed file.
//
// Parameters:
//   - `node`: the parsed file.
//   - `name`: the function-type name to find.
//
// Returns:
//   - `CallableResult`: the populated result when found.
//   - `bool`: true when the named function type was found.
func callableInFile(node *ast.File, name string) (CallableResult, bool) {

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}

			ft, ok := ts.Type.(*ast.FuncType)
			if !ok {
				continue
			}

			return CallableResult{
				Name:    name,
				Doc:     commentGroupRaw(typeSpecDoc(ts, genDecl)),
				Params:  funcTypeParams(ft),
				Returns: returnTypeString(ft.Results),
			}, true
		}
	}

	return CallableResult{}, false
}

// funcTypeParams builds the params list for a function type, expanding grouped names.
//
// Parameters:
//   - `ft`: the function type.
//
// Returns:
//   - `[]ParamDetail`: one entry per declared name (or per unnamed param).
func funcTypeParams(ft *ast.FuncType) []ParamDetail {

	params := []ParamDetail{}
	if ft.Params == nil {
		return params
	}

	for _, field := range ft.Params.List {
		typeStr := typeToString(field.Type)
		if len(field.Names) == 0 {
			params = append(params, ParamDetail{Type: typeStr})
			continue
		}
		for _, ident := range field.Names {
			params = append(params, ParamDetail{Name: ident.Name, Type: typeStr})
		}
	}

	return params
}

// typeSpecDoc picks a type spec's doc comment: TypeSpec.Doc preferred, GenDecl.Doc as the fallback.
//
// Callers render it via commentGroupRaw to preserve directive lines.
//
// Parameters:
//   - `ts`: the type spec.
//   - `genDecl`: the enclosing declaration.
//
// Returns:
//   - `*ast.CommentGroup`: the chosen doc comment, or nil when neither carries one.
func typeSpecDoc(ts *ast.TypeSpec, genDecl *ast.GenDecl) *ast.CommentGroup {

	if ts.Doc != nil {
		return ts.Doc
	}
	if genDecl.Doc != nil {
		return genDecl.Doc
	}

	return nil
}

// typeDocInFile finds the named type declaration's doc comment in one parsed file.
//
// Parameters:
//   - `node`: the parsed file.
//   - `name`: the type name to find.
//
// Returns:
//   - `string`: the raw doc comment when found.
//   - `bool`: true when the named type was found.
func typeDocInFile(node *ast.File, name string) (string, bool) {

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}

			return commentGroupRaw(typeSpecDoc(ts, genDecl)), true
		}
	}

	return "", false
}

// callResultFromExpr builds a CallResult from one call expression, applying the name filter.
//
// Parameters:
//   - `fileSet`: the file set for line positions.
//   - `call`: the call expression.
//   - `nameFilter`: when non-empty, only calls to this function name match.
//
// Returns:
//   - `CallResult`: the populated result.
//   - `bool`: true when the call names a function and passes the filter.
func callResultFromExpr(fileSet *token.FileSet, call *ast.CallExpr, nameFilter string) (CallResult, bool) {

	var funcName, qualifier, fullName string
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		funcName = fn.Name
		fullName = fn.Name
	case *ast.SelectorExpr:
		funcName = fn.Sel.Name
		if x, ok := fn.X.(*ast.Ident); ok {
			qualifier = x.Name
		}
		fullName = typeToString(call.Fun)
	}

	if funcName == "" || (nameFilter != "" && funcName != nameFilter) {
		return CallResult{}, false
	}

	return CallResult{
		Name:      funcName,
		Qualifier: qualifier,
		FullName:  fullName,
		Line:      fileSet.Position(call.Pos()).Line,
		Args:      callArgsFrom(call.Args),
	}, true
}

// callArgsFrom builds the CallArg list for a call's arguments, capturing string literals and
// identifier names.
//
// Parameters:
//   - `exprs`: the call's argument expressions.
//
// Returns:
//   - `[]CallArg`: one entry per argument, in position order.
func callArgsFrom(exprs []ast.Expr) []CallArg {

	args := []CallArg{}
	for i, arg := range exprs {
		strVal := ""
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			strVal = strings.Trim(lit.Value, `"`)
		}

		identName := ""
		switch a := arg.(type) {
		case *ast.Ident:
			identName = a.Name
		case *ast.SelectorExpr:
			identName = a.Sel.Name
		}

		args = append(args, CallArg{
			Position:    i,
			StringValue: strVal,
			IdentName:   identName,
		})
	}

	return args
}

// compositeFields extracts a composite literal's keyed fields: string literals unquoted, nested
// composites flattened to element strings, everything else rendered as its type string.
//
// Parameters:
//   - `elts`: the composite literal's elements.
//
// Returns:
//   - `map[string]any`: field name to extracted value.
func compositeFields(elts []ast.Expr) map[string]any {

	fields := map[string]any{}
	for _, elt := range elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		fields[key.Name] = compositeFieldValue(kv.Value)
	}

	return fields
}

// compositeFieldValue extracts one composite field's value per the compositeFields rules.
//
// Parameters:
//   - `value`: the field's value expression.
//
// Returns:
//   - `any`: the extracted value.
func compositeFieldValue(value ast.Expr) any {

	switch v := value.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"`)
		}
		return v.Value
	case *ast.CompositeLit:
		elems := []string{}
		for _, elem := range v.Elts {
			if lit, ok := elem.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				elems = append(elems, strings.Trim(lit.Value, `"`))
			} else {
				elems = append(elems, typeToString(elem))
			}
		}
		return elems
	default:
		return typeToString(value)
	}
}

// constEntry is one named constant within a typed const group.
type constEntry struct {
	name  string
	value string
	line  int
}

// constGroup is one contiguous run of constants sharing a declared type within a const block.
type constGroup struct {
	typeName string
	file     string
	consts   []constEntry
}

// constGroupsFromDecl walks one const declaration, splitting it into typed groups: a run of specs
// sharing a declared type forms a group, flushed when the type changes or the declaration ends;
// untyped leading specs belong to no group.
//
// Parameters:
//   - `fileSet`: the file set for line positions.
//   - `genDecl`: the const declaration.
//   - `file`: the file's base name, stamped onto each group.
//   - `typeName`: when non-empty, only groups of this type are kept.
//
// Returns:
//   - `[]constGroup`: the declaration's typed groups, in order.
func constGroupsFromDecl(fileSet *token.FileSet, genDecl *ast.GenDecl, file, typeName string) []constGroup {

	var groups []constGroup
	var currentType string
	var currentConsts []constEntry

	flush := func() {
		if currentType != "" && len(currentConsts) > 0 && (typeName == "" || typeName == currentType) {
			groups = append(groups, constGroup{typeName: currentType, file: file, consts: currentConsts})
		}
		currentConsts = nil
	}

	for _, spec := range genDecl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		if vs.Type != nil {
			if ident, ok := vs.Type.(*ast.Ident); ok {
				if currentType != "" && currentType != ident.Name {
					flush()
				}
				currentType = ident.Name
			}
		}

		if currentType == "" {
			continue
		}

		currentConsts = append(currentConsts, constEntriesFromSpec(fileSet, vs)...)
	}

	flush()

	return groups
}

// constEntriesFromSpec extracts the named constants of one value spec, capturing basic-literal values
// unquoted.
//
// Parameters:
//   - `fileSet`: the file set for line positions.
//   - `vs`: the value spec.
//
// Returns:
//   - `[]constEntry`: one entry per declared name.
func constEntriesFromSpec(fileSet *token.FileSet, vs *ast.ValueSpec) []constEntry {

	var entries []constEntry
	for i, n := range vs.Names {
		var value string
		if i < len(vs.Values) {
			if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
				value = strings.Trim(lit.Value, `"`)
			}
		}

		entries = append(entries, constEntry{
			name:  n.Name,
			value: value,
			line:  fileSet.Position(n.Pos()).Line,
		})
	}

	return entries
}

// constGroupResults maps the internal groups to the provider's result shape.
//
// Parameters:
//   - `groups`: the collected typed groups.
//
// Returns:
//   - `[]ConstGroupResult`: the result list, in group order.
func constGroupResults(groups []constGroup) []ConstGroupResult {

	result := []ConstGroupResult{}
	for _, g := range groups {
		consts := []ConstDetail{}
		for _, c := range g.consts {
			consts = append(consts, ConstDetail{Name: c.name, Value: c.value, Line: c.line})
		}
		result = append(result, ConstGroupResult{TypeName: g.typeName, File: g.file, Constants: consts})
	}

	return result
}

// methodMatches applies the Methods filters: name, receiver type (pointer-exact when the filter has
// one, pointer-insensitive otherwise), and returns signature.
//
// Parameters:
//   - `methodName`: the declaration's method name.
//   - `typeName`: the declaration's receiver type name (pointer-prefixed when so declared).
//   - `retStr`: the declaration's rendered returns.
//   - `name`: the name filter; empty matches all.
//   - `receiverType`: the receiver filter; empty matches all.
//   - `returns`: the returns filter; empty matches all.
//
// Returns:
//   - `bool`: true when every supplied filter matches.
func methodMatches(methodName, typeName, retStr, name, receiverType, returns string) bool {

	if name != "" && methodName != name {
		return false
	}

	if receiverType != "" {
		if strings.HasPrefix(receiverType, "*") {
			if typeName != receiverType {
				return false
			}
		} else if strings.TrimPrefix(typeName, "*") != receiverType {
			return false
		}
	}

	return returns == "" || retStr == returns
}

// methodResultFor builds one MethodResult from a matched method declaration.
//
// Parameters:
//   - `file`: the source name the method was parsed from.
//   - `fileSet`: the file set for line positions.
//   - `fn`: the matched method declaration.
//   - `typeName`: the receiver type name.
//   - `retStr`: the rendered returns.
//
// Returns:
//   - `MethodResult`: the populated result.
func methodResultFor(file string, fileSet *token.FileSet, fn *ast.FuncDecl, typeName, retStr string) MethodResult {

	rawDoc := ""
	if fn.Doc != nil {
		rawDoc = commentGroupRaw(fn.Doc)
	}

	return MethodResult{
		Name:         fn.Name.Name,
		ReceiverType: typeName,
		Returns:      retStr,
		Params:       extractParams(fn.Type.Params, nil),
		TypeParams:   extractTypeParams(fn.Type.TypeParams),
		File:         file,
		Line:         fileSet.Position(fn.Pos()).Line,
		Doc:          rawDoc,
		Scope:        file + "::" + typeName + "." + fn.Name.Name,
	}
}

// declBlock is one function declaration's line extent (doc comment included) within a sort scope.
type declBlock struct {
	name      string
	startLine int // 1-indexed, inclusive
	endLine   int // 1-indexed, inclusive
}

// declBlocksInScope collects the function declarations lying wholly within the line range, each
// extended to include its doc comment.
//
// Parameters:
//   - `fileSet`: the file set for line positions.
//   - `node`: the parsed file.
//   - `startLine`: the scope's first line (1-indexed, inclusive).
//   - `endLine`: the scope's last line (1-indexed, inclusive).
//
// Returns:
//   - `[]declBlock`: the in-scope declaration blocks, in source order.
func declBlocksInScope(fileSet *token.FileSet, node *ast.File, startLine, endLine int) []declBlock {

	var blocks []declBlock
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		dStart := fileSet.Position(fn.Pos()).Line
		dEnd := fileSet.Position(fn.End()).Line

		if fn.Doc != nil {
			if docStart := fileSet.Position(fn.Doc.Pos()).Line; docStart < dStart {
				dStart = docStart
			}
		}

		if dStart < startLine || dEnd > endLine {
			continue
		}

		blocks = append(blocks, declBlock{name: fn.Name.Name, startLine: dStart, endLine: dEnd})
	}

	return blocks
}

// spliceSortedBlocks rebuilds the file content with the blocks' overall range replaced by their
// sorted texts, blank-line separated.
//
// Parameters:
//   - `lines`: the file's lines.
//   - `blocks`: the declaration blocks, already in the desired order.
//
// Returns:
//   - `string`: the rebuilt file content.
func spliceSortedBlocks(lines []string, blocks []declBlock) string {

	overallStart := blocks[0].startLine
	overallEnd := blocks[0].endLine
	var sortedParts []string

	for _, b := range blocks {
		if b.startLine < overallStart {
			overallStart = b.startLine
		}
		if b.endLine > overallEnd {
			overallEnd = b.endLine
		}
		text := strings.Join(lines[b.startLine-1:b.endLine], "\n")
		sortedParts = append(sortedParts, strings.TrimRight(text, " \t\n"))
	}

	var resultLines []string
	resultLines = append(resultLines, lines[:overallStart-1]...)
	resultLines = append(resultLines, strings.Split(strings.Join(sortedParts, "\n\n"), "\n")...)
	resultLines = append(resultLines, lines[overallEnd:]...)

	return strings.Join(resultLines, "\n")
}

// structFields extracts a struct type's field details: embedded fields flagged, json-omitted fields
// skipped, grouped names expanded, and descriptions taken from trailing then leading comments.
//
// Parameters:
//   - `st`: the struct type.
//
// Returns:
//   - `[]FieldDetail`: the field details, in declaration order.
func structFields(st *ast.StructType) []FieldDetail {

	fields := []FieldDetail{}
	for _, field := range st.Fields.List {
		fields = append(fields, fieldDetailsFrom(field)...)
	}

	return fields
}

// fieldDetailsFrom extracts one field declaration's details per the structFields rules.
//
// Parameters:
//   - `field`: the field declaration.
//
// Returns:
//   - `[]FieldDetail`: one entry per declared name (or one embedded entry); empty when json-omitted.
func fieldDetailsFrom(field *ast.Field) []FieldDetail {

	if len(field.Names) == 0 {
		fieldType := typeToString(field.Type)
		return []FieldDetail{{Name: fieldType, Type: fieldType, Embedded: true}}
	}

	jsonName := ""
	required := false
	if field.Tag != nil {
		tag := strings.Trim(field.Tag.Value, "`")
		jsonName, required = parseJSONTag(tag)
	}

	if jsonName == "-" {
		return nil
	}

	desc := ""
	if field.Comment != nil {
		desc = strings.TrimSpace(field.Comment.Text())
	} else if field.Doc != nil {
		desc = strings.TrimSpace(field.Doc.Text())
	}

	fieldType := typeToString(field.Type)
	details := make([]FieldDetail, 0, len(field.Names))
	for _, ident := range field.Names {
		details = append(details, FieldDetail{
			Name:        ident.Name,
			JSONName:    jsonName,
			Type:        fieldType,
			Required:    required,
			Description: desc,
		})
	}

	return details
}

// mapKeys returns the keys of a map as a string slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
