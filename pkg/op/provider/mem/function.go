// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"reflect"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// Function is a [Resource] that holds a starlark function extracted into a self-contained synthetic source
// file; the source text and its compiled bytecode are archived in the [op.RecoverySite] as a single packed
// file (see [writeFunctionPack] for the layout).
//
// The URI is opaque: mem:function/<FuncType>/<Name>. FuncType is the named Go type the function satisfies
// (e.g., "file.Reducer", "Predicate"). Name is the function name or <action>.<param> for lambdas.
//
// Compiled and CompilerVersion are in-memory caches populated by [NewFunction] and repopulated by
// [Function.Init] when it reads bytecode out of the pack. They are NOT persisted through JSON/YAML —
// the archived pack is the persistent source of truth, and fresh Init calls repopulate the caches.
//
// Lifecycle:
//  1. [NewFunction](ctx, ResourceSpec{Data: *starlark.Function}) extracts metadata, synthesizes source,
//     compiles it, packs source+compiled into one RecoverySite entry, populates in-memory caches.
//  2. [Function.Init](thread) returns a live [starlark.Callable]. Fast path uses the in-memory Compiled
//     cache when the compiler version matches; otherwise reads the pack, and on compiler-version match
//     loads bytecode, on mismatch recompiles the source and refreshes the caches.
type Function struct {
	Resource

	// Compiled is the starlark bytecode cached in-memory. Not persisted — the pack in RecoverySite carries
	// the canonical bytes, and Init rehydrates this cache from the pack.
	Compiled []byte `json:"-" yaml:"-"`

	// CompilerVersion is [starlark.CompilerVersion] at the time Compiled was produced. Not persisted;
	// paired with the in-memory cache.
	CompilerVersion uint32 `json:"-" yaml:"-"`

	// Extraction metadata — persisted.
	FuncName    string   // function name in synthetic file (original name or "_lambda")
	ParamNames  []string // parameter names
	NumParams   int      // total params (for validation)
	OriginalPos string   // "recipe.star:42" (diagnostics only)
}

// NewFunction constructs a Function by extracting and compiling a *starlark.Function.
//
// The value must be a [ResourceSpec] with ContentType "function", Namespace encoding the Go func type, and
// Data holding a *starlark.Function. NewFunction:
//
//  1. Extracts metadata (parameter names, position, synthetic function name).
//  2. Synthesizes a self-contained source file via [synthesize].
//  3. Compiles the source via [starlark.SourceProgramOptions].
//  4. Serializes the compiled Program via [starlark.Program.Write].
//  5. Packs source + compiled + compiler version via [writeFunctionPack].
//  6. Writes the pack to the Function's URI-derived SourcePath via [op.Root.WriteFile].
//  7. Caches the compiled bytes and compiler version on the Function for in-memory fast-path Init.
//
// Parameters:
//   - ctx:   execution context; must have a valid Root.
//   - value: a [ResourceSpec] whose Data holds a *starlark.Function.
//
// Returns:
//   - *Function: the fully-populated Function.
//   - error:     if the spec is malformed, source synthesis / compilation fails, or archival fails.
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

	spec.ContentType = "function"

	// Introspect parameters.

	params := make([]string, starFn.NumParams())
	for i := range starFn.NumParams() {
		p, _ := starFn.Param(i)
		params[i] = p
	}

	base, err := op.NewResourceBase(ctx, spec.URI(), reflect.TypeFor[*Function]())
	if err != nil {
		return nil, fmt.Errorf("mem.Function: %w", err)
	}

	f := &Function{
		Resource: Resource{
			ResourceBase: base,
			ContentType:  spec.ContentType,
			Namespace:    spec.Namespace,
			Name:         spec.Name,
		},
		ParamNames: params,
		NumParams:  starFn.NumParams(),
	}

	if starFn.Name() != "lambda" {
		f.FuncName = starFn.Name()
	}

	if pos := starFn.Position(); pos.IsValid() {
		f.OriginalPos = pos.String()
	}

	// Synthesize self-contained source.

	source, err := synthesize(starFn, params)
	if err != nil {
		return nil, fmt.Errorf("mem.Function: extract %s: %w", spec.Name, err)
	}

	// Compile to bytecode.

	prog, err := compileSource(source)
	if err != nil {
		return nil, fmt.Errorf("mem.Function: compile %s: %w", spec.Name, err)
	}

	compiled, err := programToBytes(prog)
	if err != nil {
		return nil, fmt.Errorf("mem.Function: serialize %s: %w", spec.Name, err)
	}

	// Pack source + compiled + compiler version.

	var packBuf bytes.Buffer
	if err := writeFunctionPack(&packBuf, source, compiled, starlark.CompilerVersion); err != nil {
		return nil, fmt.Errorf("mem.Function: pack %s: %w", spec.Name, err)
	}

	// Derive SourcePath from URI (inherited from embedded Resource) and write the pack directly. The URI
	// is reachability; no RecoverySite UUID indirection.
	f.SourcePath, err = sourcePathFromURI(ctx.Root, f.URI())
	if err != nil {
		return nil, fmt.Errorf("mem.Function: source path: %w", err)
	}

	parentRel := filepath.Dir(f.SourcePath.Rel())
	if err := ctx.Root.MkdirAll(ctx.Root.NewPath(parentRel), 0o700); err != nil {
		return nil, fmt.Errorf("mem.Function: create parent dir: %w", err)
	}

	if err := ctx.Root.WriteFile(f.SourcePath, packBuf.Bytes(), 0o600); err != nil {
		return nil, fmt.Errorf("mem.Function: write pack %s: %w", spec.Name, err)
	}

	// Hash records the canonical source text (not the pack).
	h := sha256.Sum256(source)
	f.Hash = hex.EncodeToString(h[:])

	// In-memory caches — not persisted.
	f.Compiled = compiled
	f.CompilerVersion = starlark.CompilerVersion

	return f, nil
}

// region EXPORTED METHODS

// region Behaviors

// Init loads the compiled program, executes its toplevel, and returns the named function as a callable.
//
// Fast path: if [Function.Compiled] is non-empty and [Function.CompilerVersion] matches the runtime's
// [starlark.CompilerVersion], the program loads directly from the in-memory cache. This is the common case
// within a process.
//
// Fallback path: opens the pack from RecoverySite via mmap, inspects the header, and either (a) loads
// bytecode from the compiled section when the compiler version matches, or (b) reads the source section,
// recompiles, and caches the new bytecode on the Function. Both sub-paths refresh the in-memory Compiled
// cache so subsequent Init calls in the same process stay on the fast path.
//
// Parameters:
//   - thread: the starlark thread for program initialization.
//
// Returns:
//   - starlark.Callable: the live function.
//   - error:              non-nil if loading, compiling, or initialization fails.
func (f *Function) Init(thread *starlark.Thread) (starlark.Callable, error) {

	prog, err := f.loadProgram()
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

// Convert implements [op.SourceConverter].
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

// loadProgram returns a compiled [starlark.Program] for this Function.
//
// Checks the in-memory Compiled cache first. On miss or version mismatch, opens the pack from RecoverySite
// via mmap, reads the header, and either loads bytecode from the compiled section (version match) or
// recompiles from the source section (version mismatch or no compiled payload). Refreshes the in-memory
// cache on every fallback-path success so subsequent calls hit the fast path.
func (f *Function) loadProgram() (*starlark.Program, error) {

	// Fast path: cached in-memory bytecode matches current compiler version.
	if len(f.Compiled) > 0 && f.CompilerVersion == starlark.CompilerVersion {
		prog, err := starlark.CompiledProgram(bytes.NewReader(f.Compiled))
		if err != nil {
			return nil, fmt.Errorf("load cached bytecode: %w", err)
		}
		return prog, nil
	}

	// Fallback: open the pack from the URI-derived SourcePath.

	abs := f.SourcePath.Abs()
	if abs == "" {
		return nil, fmt.Errorf("no SourcePath — Function was not archived")
	}

	m, err := mmap.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("mmap %s: %w", abs, err)
	}
	defer func() { _ = m.Close() }()

	h, err := readFunctionPackHeader(m)
	if err != nil {
		return nil, err
	}

	if h.CompiledSize > 0 && h.CompilerVersion == starlark.CompilerVersion {

		compiledBytes := make([]byte, h.CompiledSize)
		if _, err := m.ReadAt(compiledBytes, int64(h.CompiledOffset)); err != nil {
			return nil, fmt.Errorf("read compiled section: %w", err)
		}

		prog, err := starlark.CompiledProgram(bytes.NewReader(compiledBytes))
		if err != nil {
			return nil, fmt.Errorf("decode compiled: %w", err)
		}

		f.Compiled = compiledBytes
		f.CompilerVersion = h.CompilerVersion

		return prog, nil
	}

	// Compiler-version mismatch or no compiled payload: recompile from source section.

	source, err := io.ReadAll(sourceReader(m, h))
	if err != nil {
		return nil, fmt.Errorf("read source section: %w", err)
	}

	prog, err := compileSource(source)
	if err != nil {
		return nil, fmt.Errorf("recompile: %w", err)
	}

	// Cache the freshly-compiled bytes for subsequent in-process Init calls.
	if compiled, cerr := programToBytes(prog); cerr == nil {
		f.Compiled = compiled
		f.CompilerVersion = starlark.CompilerVersion
	}

	return prog, nil
}

// endregion

// endregion

// compileSource parses and compiles the given starlark source text.
//
// Parameters:
//   - source: the UTF-8 source text of a self-contained synthetic file.
//
// Returns:
//   - *starlark.Program: the compiled program, ready for Init.
//   - error:              any parse or compile error.
func compileSource(source []byte) (*starlark.Program, error) {

	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, "<function>", source, func(string) bool { return false },
	)
	return prog, err
}

// programToBytes serializes a compiled program via [starlark.Program.Write].
//
// Parameters:
//   - prog: the compiled program.
//
// Returns:
//   - []byte: the serialized bytecode, suitable for [starlark.CompiledProgram].
//   - error:  any error from Program.Write.
func programToBytes(prog *starlark.Program) (_ []byte, err error) {

	var buf bytes.Buffer

	if err := prog.Write(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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
