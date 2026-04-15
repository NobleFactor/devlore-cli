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

var errorType = reflect.TypeFor[error]()

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
			&syntax.FileOptions{},
			"<function>",
			f.Data, func(string) bool { return false },
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

// Convert implements [op.Convertible].
//
// Converts to any Go func type by building a bridge that marshals arguments, calls the underlying starlark function,
// and unmarshals the result. The starlark function's parameter count must match the Go func's input count. Varargs and
// kwargs are rejected. The Go func may return (), (T), (error), or (T, error).
//
// Parameters:
//   - target: the Go func type to convert to.
//
// Returns:
//   - any: a Go function of the target type.
//   - error: non-nil if the target is not a func type or the signature doesn't match.
func (f *Function) ConvertTo(target reflect.Type) (any, error) {

	if target.Kind() != reflect.Func {
		return nil, fmt.Errorf("mem.Function: cannot convert to %s (not a func type)", target)
	}

	// Initialize the callable.

	ctx := f.ExecutionContext()
	thread := &ctx.Thread

	callable, err := f.Init(thread)
	if err != nil {
		return nil, fmt.Errorf("mem.Function: init: %w", err)
	}

	// Validate signature.

	starFn, ok := callable.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("mem.Function: callable is %T, expected *starlark.Function", callable)
	}

	if starFn.NumParams() != target.NumIn() {
		return nil, fmt.Errorf("mem.Function: param count mismatch: starlark %d, Go %d",
			starFn.NumParams(), target.NumIn())
	}

	if starFn.HasVarargs() || starFn.HasKwargs() {
		return nil, fmt.Errorf("mem.Function: starlark function uses *args/**kwargs, cannot bridge to fixed Go signature")
	}

	hasError := target.NumOut() > 0 && target.Out(target.NumOut()-1).Implements(errorType)
	numValues := target.NumOut()
	if hasError {
		numValues--
	}

	if numValues > 1 {
		return nil, fmt.Errorf("mem.Function: Go func returns %d non-error values, max 1", numValues)
	}

	// Build bridge function.

	bridge := reflect.MakeFunc(target, func(args []reflect.Value) []reflect.Value {

		starArgs := make(starlark.Tuple, len(args))

		for i, arg := range args {

			sv, merr := goToStarlark(arg)

			if merr != nil {
				return funcError(target, numValues, hasError, fmt.Errorf("arg %d: %w", i, merr))
			}

			starArgs[i] = sv
		}

		result, cerr := starlark.Call(thread, callable, starArgs, nil)
		if cerr != nil {
			return funcError(target, numValues, hasError, cerr)
		}

		return funcReturn(target, numValues, hasError, result)
	})

	return bridge.Interface(), nil
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

// funcError builds the return slice for a failed starlark call.
//
// If the Go func has an error return, the error occupies the last position and value slots are zeroed. If the Go func
// has no error return, funcError panics — the caller chose a signature that cannot report errors.
func funcError(target reflect.Type, numValues int, hasError bool, err error) []reflect.Value {

	if !hasError {
		panic(fmt.Sprintf("starlark bridge: %v", err))
	}

	out := make([]reflect.Value, target.NumOut())

	for i := range numValues {
		out[i] = reflect.Zero(target.Out(i))
	}

	out[len(out)-1] = reflect.ValueOf(&err).Elem()

	return out
}

// funcReturn builds the return slice for a successful starlark call.
func funcReturn(target reflect.Type, numValues int, hasError bool, result starlark.Value) []reflect.Value {

	out := make([]reflect.Value, target.NumOut())

	if numValues == 1 {

		goVal, err := starlarkToGo(result)

		if err != nil {
			return funcError(target, numValues, hasError, fmt.Errorf("return: %w", err))
		}

		out[0] = reflect.ValueOf(goVal).Convert(target.Out(0))
	}

	if hasError {
		out[len(out)-1] = reflect.Zero(target.Out(len(out) - 1))
	}

	return out
}

// goToStarlark converts a [reflect.Value] to a [starlark.Value].
//
// Parameters:
//   - rv: the Go value to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark value.
//   - error: non-nil if the type is unsupported.
func goToStarlark(rv reflect.Value) (starlark.Value, error) {

	if !rv.IsValid() {
		return starlark.None, nil
	}

	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {

		if rv.IsNil() {
			return starlark.None, nil
		}

		rv = rv.Elem()
	}

	switch rv.Kind() {

	case reflect.String:
		return starlark.String(rv.String()), nil

	case reflect.Bool:
		return starlark.Bool(rv.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(rv.Int()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(rv.Uint()), nil

	case reflect.Float32, reflect.Float64:
		return starlark.Float(rv.Float()), nil

	case reflect.Slice:

		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return starlark.Bytes(rv.Bytes()), nil
		}

		return nil, fmt.Errorf("goToStarlark: unsupported slice type %s", rv.Type())

	default:
		return nil, fmt.Errorf("goToStarlark: unsupported type %s", rv.Type())
	}
}

// starlarkToGo converts a [starlark.Value] to a native Go value.
//
// Parameters:
//   - sv: the Starlark value to convert.
//
// Returns:
//   - any: the native Go value.
//   - error: non-nil if the type is unsupported.
func starlarkToGo(sv starlark.Value) (any, error) {

	switch v := sv.(type) {

	case starlark.NoneType:
		return nil, nil

	case starlark.String:
		return string(v), nil

	case starlark.Int:

		i, ok := v.Int64()

		if !ok {
			return nil, fmt.Errorf("starlarkToGo: int value out of range")
		}

		return int(i), nil

	case starlark.Bool:
		return bool(v), nil

	case starlark.Float:
		return float64(v), nil

	case starlark.Bytes:
		return []byte(v), nil

	default:
		return nil, fmt.Errorf("starlarkToGo: unsupported starlark type %s", sv.Type())
	}
}

// endregion

// endregion
