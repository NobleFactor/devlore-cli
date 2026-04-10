// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func init() {
	op.AnnounceResource(
		reflect.TypeFor[Function](),
		func(ctx *op.ExecutionContext, value any) (any, error) {
			return NewFunction(ctx, value)
		},
		nil,
	)
}

// Function is a mem.Resource that holds a starlark function extracted into a self-contained synthetic source file with
// compiled bytecode.
//
// The URI is opaque: mem:function/<FuncType>/<Name>. FuncType is the named Go type the function satisfies (e.g.,
// "file.Reducer", "Predicate"). Name is the function name or <action>.<param> for lambdas.
//
// The Data field (inherited from Resource) holds the synthetic source text. The Compiled field holds serialized bytecode
// from Program.Write. Both are []byte — serializable, transferable, persistable.
//
// Lifecycle:
//  1. NewFunction(ctx, uri, *starlark.Function) → extracts and compiles
//  2. Init(thread) → loads bytecode, extracts live fn
//  3. Fn() → returns live callable for adapter invocation
type Function struct {
	Resource // embeds mem.Resource (source text in Data)

	Compiled []byte // bytecode — Program.Write output, recompiled on version mismatch

	// Extraction metadata.
	FuncName        string   // function name in synthetic file ("_callable" or original)
	ParamNames      []string // parameter names
	NumParams       int      // total params (for validation)
	CompilerVersion uint32   // starlark.CompilerVersion at compile time
	OriginalPos     string   // "recipe.star:42" (diagnostics only)
}

// NewFunction constructs a Function by extracting and compiling a *starlark.Function.
//
// The value must be a [ResourceSpec] with ContentType "function", Qualifier encoding the func type and name
// (e.g., "file.Reducer/count_python_files"), and Data holding a *starlark.Function.
//
// Parameters:
//   - ctx: execution context.
//   - value: a [ResourceSpec] with Data holding a *starlark.Function.
//
// Returns:
//   - *Function: the extracted and compiled function.
//   - error: if extraction or compilation fails.
func NewFunction(ctx *op.ExecutionContext, value any) (*Function, error) {

	spec, ok := value.(ResourceSpec)
	if !ok {
		return nil, fmt.Errorf("mem.Function: expected ResourceSpec, got %T", value)
	}

	starFn, ok := spec.Data.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("mem.Function: expected *starlark.Function in Data, got %T", spec.Data)
	}

	if spec.Namespace == "" {
		return nil, fmt.Errorf("mem.Function: empty namespace (func type)")
	}
	if spec.Name == "" {
		spec.Name = starFn.Name()
		if spec.Name == "lambda" {
			spec.Name = "_lambda"
		}
	}

	// Build the Function.

	spec.ContentType = "function"

	f := &Function{
		Resource: Resource{
			ResourceBase: op.NewResourceBase(ctx, spec.URI()),
			ContentType:  spec.ContentType,
			Namespace:    spec.Namespace,
			Name:         spec.Name,
		},
		FuncName: "_callable",
	}

	// Introspect parameters.

	params := make([]string, starFn.NumParams())
	for i := range starFn.NumParams() {
		p, _ := starFn.Param(i)
		params[i] = p
	}
	f.ParamNames = params
	f.NumParams = starFn.NumParams()

	// Record original position for diagnostics.

	if pos := starFn.Position(); pos.IsValid() {
		f.OriginalPos = pos.String()
	}

	// Set function name in synthetic file.

	if starFn.Name() != "lambda" {
		f.FuncName = starFn.Name()
	}

	// Synthesize self-contained source.

	source, err := synthesize(starFn, params)
	if err != nil {
		return nil, fmt.Errorf("mem.Function: extract %s: %w", spec.Name, err)
	}
	f.Data = source
	f.ComputeHash()

	// Compile to bytecode.

	if err := f.compile(); err != nil {
		return nil, err
	}

	return f, nil
}

// region EXPORTED METHODS

// region Behaviors

// Init loads the compiled program (or recompiles from source on version mismatch) and returns the callable function.
//
// Each call returns a fresh callable with its own program globals — safe for concurrent use by gather iterations.
//
// Parameters:
//   - thread: the starlark thread for program initialization.
//
// Returns:
//   - starlark.Callable: the live function.
//   - error: non-nil if loading or initialization fails.
func (f *Function) Init(thread *starlark.Thread) (starlark.Callable, error) {

	var prog *starlark.Program
	var err error

	if len(f.Compiled) > 0 && f.CompilerVersion == starlark.CompilerVersion {
		prog, err = starlark.CompiledProgram(bytes.NewReader(f.Compiled))
	} else {
		_, prog, err = starlark.SourceProgramOptions(
			&syntax.FileOptions{}, "<function>", f.Data, func(string) bool { return false },
		)
	}
	if err != nil {
		return nil, fmt.Errorf("mem.Function init: %w", err)
	}

	globals, err := prog.Init(thread, nil)
	if err != nil {
		return nil, fmt.Errorf("mem.Function init: %w", err)
	}

	fn, ok := globals[f.FuncName]
	if !ok {
		return nil, fmt.Errorf("mem.Function init: function %q not found", f.FuncName)
	}
	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("mem.Function init: %q is %s, not callable", f.FuncName, fn.Type())
	}
	return callable, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// compile compiles the synthetic source text and stores the bytecode.
//
// Returns:
//   - error: non-nil if compilation fails.
func (f *Function) compile() error {

	if len(f.Data) == 0 {
		return fmt.Errorf("mem.Function compile: no source text")
	}
	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, "<function>", f.Data, func(string) bool { return false },
	)
	if err != nil {
		return fmt.Errorf("mem.Function compile: %w", err)
	}
	var buf bytes.Buffer
	if err := prog.Write(&buf); err != nil {
		return fmt.Errorf("mem.Function compile write: %w", err)
	}
	f.Compiled = buf.Bytes()
	f.CompilerVersion = starlark.CompilerVersion
	return nil
}

// endregion

// endregion
