// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

// =============================================================================
// METRICS RESULT TYPES
// =============================================================================

// MetricsResult holds aggregated code metrics for a path.
type MetricsResult struct {
	Files              []FileMetric `starlark:"files"`
	FileCount          int          `starlark:"file_count"`
	TotalLOC           int          `starlark:"total_loc"`
	TotalSLOC          int          `starlark:"total_sloc"`
	TotalComments      int          `starlark:"total_comments"`
	TotalBlanks        int          `starlark:"total_blanks"`
	TotalFunctions     int          `starlark:"total_functions"`
	TotalMethods       int          `starlark:"total_methods"`
	TotalStructs       int          `starlark:"total_structs"`
	TotalInterfaces    int          `starlark:"total_interfaces"`
	TotalTypes         int          `starlark:"total_types"`
	TotalConstants     int          `starlark:"total_constants"`
	TotalVariables     int          `starlark:"total_variables"`
	TotalImports       int          `starlark:"total_imports"`
	TotalTestFunctions int          `starlark:"total_test_functions"`
}

// FileMetric holds code metrics for a single file.
type FileMetric struct {
	Path          string `starlark:"path"`
	LOC           int    `starlark:"loc"`
	SLOC          int    `starlark:"sloc"`
	Comments      int    `starlark:"comments"`
	Blanks        int    `starlark:"blanks"`
	Functions     int    `starlark:"functions"`
	Methods       int    `starlark:"methods"`
	Structs       int    `starlark:"structs"`
	Interfaces    int    `starlark:"interfaces"`
	Types         int    `starlark:"types"`
	Constants     int    `starlark:"constants"`
	Variables     int    `starlark:"variables"`
	Imports       int    `starlark:"imports"`
	TestFunctions int    `starlark:"test_functions"`
}

// =============================================================================
// DEPENDENCY RESULT TYPES
// =============================================================================

// DepsResult holds aggregated dependency information for a path.
type DepsResult struct {
	Files         []FileDep `starlark:"files"`
	ModulePath    string    `starlark:"module_path"`
	AllImports    []string  `starlark:"all_imports"`
	InternalDeps  []string  `starlark:"internal_deps"`
	ExternalDeps  []string  `starlark:"external_deps"`
	StdlibDeps    []string  `starlark:"stdlib_deps"`
	InternalCount int       `starlark:"internal_count"`
	ExternalCount int       `starlark:"external_count"`
	StdlibCount   int       `starlark:"stdlib_count"`
}

// FileDep holds dependency information for a single file.
type FileDep struct {
	Path         string         `starlark:"path"`
	Package      string         `starlark:"package"`
	Imports      []ImportDetail `starlark:"imports"`
	InternalDeps []string       `starlark:"internal_deps"`
	ExternalDeps []string       `starlark:"external_deps"`
	StdlibDeps   []string       `starlark:"stdlib_deps"`
}

// ImportDetail holds information about a single import.
type ImportDetail struct {
	Path  string `starlark:"path"`
	Alias string `starlark:"alias"`
	Line  int    `starlark:"line"`
}

// =============================================================================
// CALLABLE RESULT TYPE
// =============================================================================

// CallableResult holds information about a function type declaration.
type CallableResult struct {
	Name    string        `starlark:"name"`
	Doc     string        `starlark:"doc"`
	Params  []ParamDetail `starlark:"params"`
	Returns string        `starlark:"returns"`
}

// ParamDetail holds information about a function parameter.
type ParamDetail struct {
	Name     string `starlark:"name"`
	Type     string `starlark:"type"`
	Variadic bool   `starlark:"variadic"`
	Doc      string `starlark:"doc"`
}

// TypeParamDetail holds a generic type parameter and its constraint's type-set members.
//
// For `[T *starlark.Function | string]`, Name is "T" and Constraint is ["*starlark.Function", "string"]. A non-union
// constraint such as `[T any]` yields a single member ("any"); an approximation term `~int` keeps its `~` prefix.
type TypeParamDetail struct {
	Name       string   `starlark:"name"`
	Constraint []string `starlark:"constraint"`
}

// =============================================================================
// STRUCT RESULT TYPE
// =============================================================================

// StructResult holds information about a struct type declaration.
type StructResult struct {
	Name   string        `starlark:"name"`
	File   string        `starlark:"file"`
	Line   int           `starlark:"line"`
	Fields []FieldDetail `starlark:"fields"`
}

// FieldDetail holds information about a struct field.
type FieldDetail struct {
	Name        string `starlark:"name"`
	JSONName    string `starlark:"json_name"`
	Type        string `starlark:"type"`
	Required    bool   `starlark:"required"`
	Description string `starlark:"description"`
	Embedded    bool   `starlark:"embedded"`
}

// =============================================================================
// CONST GROUP RESULT TYPE
// =============================================================================

// ConstGroupResult holds information about a typed const group.
type ConstGroupResult struct {
	TypeName  string        `starlark:"type_name"`
	File      string        `starlark:"file"`
	Constants []ConstDetail `starlark:"constants"`
}

// ConstDetail holds information about a single constant.
type ConstDetail struct {
	Name  string `starlark:"name"`
	Value string `starlark:"value"`
	Line  int    `starlark:"line"`
}

// =============================================================================
// METHOD/FUNCTION RESULT TYPES
// =============================================================================

// MethodResult holds information about a method declaration.
type MethodResult struct {
	Name         string            `starlark:"name"`
	ReceiverType string            `starlark:"receiver_type"`
	Returns      string            `starlark:"returns"`
	Params       []ParamDetail     `starlark:"params"`
	TypeParams   []TypeParamDetail `starlark:"type_params"`
	File         string            `starlark:"file"`
	Line         int               `starlark:"line"`
	Doc          string            `starlark:"doc"`
	Scope        string            `starlark:"scope"`
}

// FuncResult holds information about a top-level function declaration.
type FuncResult struct {
	Name       string            `starlark:"name"`
	Returns    string            `starlark:"returns"`
	Params     []ParamDetail     `starlark:"params"`
	TypeParams []TypeParamDetail `starlark:"type_params"`
	File       string            `starlark:"file"`
	Line       int               `starlark:"line"`
	Doc        string            `starlark:"doc"`
	Scope      string            `starlark:"scope"`
}

// =============================================================================
// CALL RESULT TYPE
// =============================================================================

// CallResult holds information about a function/method call.
type CallResult struct {
	Name      string    `starlark:"name"`
	Qualifier string    `starlark:"qualifier"`
	FullName  string    `starlark:"full_name"`
	Line      int       `starlark:"line"`
	Args      []CallArg `starlark:"args"`
}

// CallArg holds information about a call argument.
type CallArg struct {
	Position    int    `starlark:"position"`
	StringValue string `starlark:"string_value"`
	IdentName   string `starlark:"ident_name"`
}

// =============================================================================
// COMPOSITE RESULT TYPE
// =============================================================================

// CompositeResult holds information about a composite literal.
type CompositeResult struct {
	TypeName string         `starlark:"type_name"`
	Line     int            `starlark:"line"`
	Fields   map[string]any `starlark:"fields"`
}

// =============================================================================
// LINE WIDTH VIOLATIONS
// =============================================================================

// LineViolation holds a single line-width check result.
type LineViolation struct {
	Line    int    `starlark:"line"`
	Message string `starlark:"message"`
}
