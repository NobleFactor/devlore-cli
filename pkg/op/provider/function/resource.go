// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op/provider/mem"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
)

var errorType = reflect.TypeFor[error]()

// Interface Guard: *Resource implements op.Packer (overriding the embedded mem.Resource implementation).
var _ op.Packer = (*Resource)(nil)

// Interface Guard: *Resource implements op.Unpacker (overriding the embedded mem.Resource implementation).
var _ op.Unpacker = (*Resource)(nil)

// Resource holds a starlark function extracted into a self-contained synthetic source file.
//
// The source text and its compiled bytecode are archived in the [op.RecoverySite] as a single packed file (see
// [writeFunctionPack] for the layout).
//
// Identity is content-addressed: the URI's <specific> is `sha256:<hex>` over the synthesized source bytes. The
// on-disk path follows mem's sharded CAS formula via the embedded [mem.Resource], so the pack lives at
// <Root>/.devlore/function/resource/sha256/<hex[0:2]>/<hex>.
//
// Compiled and CompilerVersion are in-memory caches populated by [NewResource] and repopulated by [Resource.Init]
// when it reads bytecode out of the pack. They are NOT persisted through JSON/YAML — the archived pack is the
// persistent source of truth, and fresh Init calls repopulate the caches.
//
// Lifecycle:
//  1. [NewResource](activation, *starlark.Function) extracts metadata, synthesizes source, computes the source
//     digest as identity, compiles, packs source+compiled into one RecoverySite entry, populates in-memory caches.
//  2. [Resource.Init](thread) returns a live [starlark.Callable]. Fast path uses the in-memory Compiled cache when
//     the compiler version matches; otherwise reads the pack, and on compiler-version match loads bytecode, on
//     mismatch recompiles the source and refreshes the caches.
type Resource struct {
	mem.Resource

	// invoker is this resource's env-free Go↔Starlark call surface, built at construction and used by ConvertTo to
	// route every reducer invocation through one conversion path. Unexported, so it is never serialized.
	invoker starlarkbridge.Invoker

	// Compiled is the starlark bytecode cached in-memory. Not persisted — the pack in RecoverySite carries the
	// canonical bytes, and Init rehydrates this cache from the pack.
	Compiled []byte `json:"-" yaml:"-"`

	// CompilerVersion is [starlark.CompilerVersion] at the time Compiled was produced. Not persisted; paired with
	// the in-memory cache.
	CompilerVersion uint32 `json:"-" yaml:"-"`

	// FuncName is the function name in the synthetic file (the original name, or "_lambda" for anonymous defs).
	// Persisted in the in-memory marshaled shape but not load-bearing for identity.
	FuncName string

	// ParamNames is the ordered list of parameter names extracted from the original function.
	ParamNames []string

	// NumParams is the total parameter count (for validation against bridge target signatures).
	NumParams int

	// OriginalPos is the source position the function was extracted from (diagnostics only, e.g.,
	// "recipe.star:42").
	OriginalPos string
}

// NewResource constructs a *Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped with
// `producerID = activationRecord.CallerID.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles, the
// framework's slot-coercion adapter).
//
// Identity is the SHA-256 of the synthesized source bytes. When identity is a *starlark.Function, NewResource:
//
//  1. Introspects parameters and metadata.
//  2. Synthesizes a self-contained source file via [synthesize].
//  3. Hashes the synthesized source bytes to obtain the canonical identity.
//  4. Compiles the source via [starlark.SourceProgramOptions].
//  5. Serializes the compiled Program via [starlark.Program.Write].
//  6. Packs source + compiled + compiler version via [writeFunctionPack].
//  7. Writes the pack to the Resource's URI-derived SourcePath (sharded CAS path).
//  8. Caches the compiled bytes and compiler version on the Resource for in-memory fast-path Init.
//
// When identity is a string URI, NewResource rehydrates a metadata-only Resource (no archival; the URI alone
// carries the source digest).
//
// Two callers with byte-identical synthesized source produce the same URI; the first to reach the catalog wins.
// The second caller's write overwrites the canonical path with byte-identical content.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment. `Root` must be non-nil when `identity` is a
//     *starlark.Function.
//   - `producerID`: the producing caller's id (`activationRecord.CallerID`), or "" for caller-less dispatch.
//     for non-graph dispatch.
//   - `identity`: a *starlark.Function (archival) or a canonical tag URI string (metadata-only rehydration).
//
// Returns:
//   - `*Resource`: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported identity type, synthesis/compilation failure, filesystem write failure, malformed URI,
//     or identity construction failure.
func NewResource[T *starlark.Function | string](
	runtimeEnvironment *op.RuntimeEnvironment,
	producerID string,
	identity T,
) (*Resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, identity)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.GetOrCreate(producerID, candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("function.NewResource: catalog entry for %q is %T, want *function.Resource",
			candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource registers a *Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string URI
// and the slot expects a *function.Resource) and by callers holding a reference handle without claiming
// production.
//
// Discover does not stamp a producer, so unlike [NewResource] it takes only `runtimeEnvironment` — no
// unit reference is needed.
//
// Same identity-shape dispatch as [NewResource]: *starlark.Function archives content; string rehydrates
// metadata-only.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `identity`: a *starlark.Function or a canonical tag URI string; same dispatch as [NewResource].
//
// Returns:
//   - `*Resource`: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported identity type, synthesis/compilation failure, filesystem write failure, malformed URI,
//     or identity construction failure.
func DiscoverResource(
	runtimeEnvironment *op.RuntimeEnvironment,
	identity any,
) (*Resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, identity)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("function.DiscoverResource: catalog entry for %q is %T, want *function.Resource",
			candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate returns an unlinked *Resource for identity.
//
// *starlark.Function values are dispatched to [newFromFunction] (extracts, compiles, packs, and archives the function).
// String URI values are dispatched to [newFromURI] (metadata-only rehydration). Resource catalog interaction is the
// caller's concern, not this function's. See [NewResource] and [DiscoverResource].
//
// Parameters:
//   - `runtimeEnvironment`: runtime environment threaded into the produced [op.ResourceBase].
//   - `identity`: a *starlark.Function or a canonical tag URI string; any other type is an error.
//
// Returns:
//   - `*Resource`: unlinked candidate.
//   - `error`: unsupported identity type, or an error from the downstream constructor.
func buildCandidate(
	runtimeEnvironment *op.RuntimeEnvironment,
	identity any,
) (*Resource, error) {

	switch v := identity.(type) {
	case *starlark.Function:
		return newFromFunction(runtimeEnvironment, v)

	case string:
		return newFromURI(runtimeEnvironment, v)

	default:
		return nil, fmt.Errorf("function.Resource: expected *starlark.Function or URI string, got %T", identity)
	}
}

// newFromFunction extracts metadata, synthesizes source, hashes it as identity, compiles to bytecode, and packs
// everything to the canonical CAS path.
//
// Parameters:
//   - `runtimeEnvironment`: supplies [fsroot.Root] for the canonical CAS path. Must have a non-nil Root.
//   - `fn`: starlark function to extract.
//
// Returns:
//   - `*Resource`: candidate with embedded [mem.Resource] keyed on the source digest, in-memory caches populated, and
//     the pack archived on disk.
//   - `error`: extraction, synthesis, compilation, serialization, identity construction, parent-directory creation, or
//     write failure.
func newFromFunction(
	runtimeEnvironment *op.RuntimeEnvironment,
	fn *starlark.Function,
) (*Resource, error) {

	if fn == nil {
		return nil, fmt.Errorf("function.Resource: nil *starlark.Function")
	}

	// Introspect parameters.

	params := make([]string, fn.NumParams())

	for i := range fn.NumParams() {
		p, _ := fn.Param(i)
		params[i] = p
	}

	// Synthesize self-contained source.

	source, err := synthesize(fn, params)
	if err != nil {
		return nil, fmt.Errorf("function.Resource: extract %s: %w", fn.Name(), err)
	}

	funcName := anonymousFuncName
	if fn.Name() != "lambda" {
		funcName = fn.Name()
	}

	var originalPos string
	if pos := fn.Position(); pos.IsValid() {
		originalPos = pos.String()
	}

	return newFromSource(runtimeEnvironment, funcName, params, originalPos, source)
}

// newFromSource builds a *Resource from already-synthesized source text: identity, compilation, and the archived
// pack.
//
// The shared tail of the two construction paths — [newFromFunction] hands it the source it synthesized from a live
// [*starlark.Function]; [Resource.Unpack] hands it the source that traveled in a graph document's content section.
// Identity is the SHA-256 of the source bytes, so both paths reproduce the same URI for the same function.
//
// Parameters:
//   - `runtimeEnvironment`: supplies [fsroot.Root] for the canonical CAS path. Must have a non-nil Root.
//   - `funcName`: the function's name in the synthetic file (or the anonymous placeholder).
//   - `paramNames`: the ordered parameter names.
//   - `originalPos`: the original `.star` source position, or "" when unknown.
//   - `source`: the self-contained synthetic source text.
//
// Returns:
//   - `*Resource`: candidate with embedded [mem.Resource] keyed on the source digest, in-memory caches populated,
//     and the pack archived on disk.
//   - `error`: compilation, serialization, identity construction, parent-directory creation, or write failure.
func newFromSource(
	runtimeEnvironment *op.RuntimeEnvironment,
	funcName string,
	paramNames []string,
	originalPos string,
	source []byte,
) (*Resource, error) {

	// Compute identity from source bytes.

	sum := sha256.Sum256(source)
	hexDigest := hex.EncodeToString(sum[:])

	base, err := op.NewResourceBase(runtimeEnvironment, "sha256:"+hexDigest, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("function.Resource: %w", err)
	}

	f := &Resource{
		Resource: mem.Resource{
			ResourceBase: base,
			Hash:         hexDigest,
		},
		invoker:     starlarkbridge.NewInvoker(),
		FuncName:    funcName,
		ParamNames:  paramNames,
		NumParams:   len(paramNames),
		OriginalPos: originalPos,
	}

	// Compile to bytecode.

	prog, err := compileSource(source)
	if err != nil {
		return nil, fmt.Errorf("function.Resource: compile %s: %w", f.FuncName, err)
	}

	compiled, err := programToBytes(prog)
	if err != nil {
		return nil, fmt.Errorf("function.Resource: serialize %s: %w", f.FuncName, err)
	}

	// Pack source + compiled + compiler version.

	var packBuf bytes.Buffer
	if err := writeFunctionPack(&packBuf, source, compiled, starlark.CompilerVersion); err != nil {
		return nil, fmt.Errorf("function.Resource: pack %s: %w", f.FuncName, err)
	}

	// Write pack to the canonical CAS path (inherited sharded SourcePath via embedded mem.Resource).

	sp := f.SourcePath()

	parentRel := filepath.Dir(sp.Rel())
	if err := runtimeEnvironment.Root.MkdirAll(runtimeEnvironment.Root.NewPath(parentRel), 0o700); err != nil {
		return nil, fmt.Errorf("function.Resource: create canonical dir: %w", err)
	}

	if err := runtimeEnvironment.Root.WriteFile(sp, packBuf.Bytes(), 0o600); err != nil {
		return nil, fmt.Errorf("function.Resource: write pack %s: %w", f.FuncName, err)
	}

	// In-memory caches — not persisted.

	f.Compiled = compiled
	f.CompilerVersion = starlark.CompilerVersion

	return f, nil
}

// newFromURI rehydrates a metadata-only *Resource from a canonical tag URI.
//
// The URI's <specific> portion must be `<algo>:<hex>` (currently only `sha256:` is supported). The digest is
// stamped on the embedded mem.Resource's Hash field. No content is archived — the on-disk pack is the source of
// truth, rehydrated lazily by [Resource.Init] / [loadProgram] via the URI-derived SourcePath.
//
// Parameters:
//   - `runtimeEnvironment`: runtime environment threaded into the produced [op.ResourceBase].
//   - `uri`: canonical tag URI; <specific> must be `<algo>:<hex>` with `algo == "sha256"`.
//
// Returns:
//   - `*Resource`: metadata-only Resource with embedded mem.Resource Hash populated.
//   - `error`: malformed URI, deferred (empty <specific>) URI, missing colon, unsupported algorithm, malformed hex,
//     or [op.ResourceBase] construction failure.
func newFromURI(runtimeEnvironment *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("function.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("function.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	algo, hexPart, ok := strings.Cut(specific, ":")
	if !ok {
		return nil, fmt.Errorf("function.Resource: URI specific %q is not in <algo>:<hex> form", specific)
	}
	if algo != "sha256" {
		return nil, fmt.Errorf("function.Resource: unsupported digest algorithm %q (want sha256)", algo)
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return nil, fmt.Errorf("function.Resource: invalid digest hex %q: %w", hexPart, err)
	}

	base, err := op.NewResourceBase(runtimeEnvironment, specific, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("function.Resource: %w", err)
	}

	return &Resource{
		Resource: mem.Resource{
			ResourceBase: base,
			Hash:         hexPart,
		},
		invoker: starlarkbridge.NewInvoker(),
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// CanConvertTo implements [op.SourceConverter].
//
// Parameters:
//   - `target`: destination Go type.
//
// Returns:
//   - `bool`: true when target is a Go func type, or when the embedded mem.Resource can convert to target.
func (f *Resource) CanConvertTo(target reflect.Type) bool {
	if reflect.TypeOf(f).AssignableTo(target) {
		return true
	}
	if target.Kind() == reflect.Func {
		return true
	}
	return f.Resource.CanConvertTo(target)
}

// ConvertTo implements [op.SourceConverter].
//
// Converts to any Go func type by building a bridge that converts arguments, calls the underlying starlark function,
// and converts the result. The starlark function's parameter count must match the Go func's input count. Varargs
// and kwargs are rejected. The Go func may return (), (T), (error), or (T, error). For non-func targets, delegates
// to the embedded mem.Resource's ConvertTo (which projects content to []byte or string).
//
// Parameters:
//   - `target`: the Go type to convert to.
//
// Returns:
//   - `any`: a Go function of the target type, or the projected content for []byte / string targets.
//   - `error`: non-nil if the target is not supported, the signature doesn't match, or the underlying call fails.
func (f *Resource) ConvertTo(target reflect.Type) (any, error) {

	if reflect.TypeOf(f).AssignableTo(target) {
		// Identity: a slot declared as the resource type itself (e.g. function.call's `callable` parameter) takes the
		// resource unconverted.
		return f, nil
	}

	if target.Kind() != reflect.Func {
		if f.Resource.CanConvertTo(target) {
			return f.Resource.ConvertTo(target)
		}
		return nil, fmt.Errorf("function.Resource: cannot convert to %s (not a func type)", target)
	}

	// Initialize the callable. Init runs the program once on its own thread; the reducer call below goes through the
	// Invoker, which mints a fresh thread per invocation.

	callable, err := f.Init(&starlark.Thread{Name: "function.Resource"})
	if err != nil {
		return nil, fmt.Errorf("function.Resource: init: %w", err)
	}

	// Validate signature.

	starFn, ok := callable.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("function.Resource: callable is %T, expected *starlark.Function", callable)
	}

	if starFn.NumParams() != target.NumIn() {
		return nil, fmt.Errorf("function.Resource: param count mismatch: starlark %d, Go %d",
			starFn.NumParams(), target.NumIn())
	}

	if starFn.HasVarargs() || starFn.HasKwargs() {
		return nil, fmt.Errorf(
			"function.Resource: starlark function uses *args/**kwargs, cannot bridge to fixed Go signature")
	}

	hasError := target.NumOut() > 0 && target.Out(target.NumOut()-1).Implements(errorType)
	numValues := target.NumOut()

	if hasError {
		numValues--
	}

	if numValues > 1 {
		return nil, fmt.Errorf("function.Resource: Go func returns %d non-error values, max 1", numValues)
	}

	// Route every reducer invocation through this resource's own Invoker — the single Go↔Starlark call surface.

	invoker := f.invoker
	if invoker == nil {
		return nil, fmt.Errorf("function.Resource: invoker not initialized on the resource")
	}

	// Build bridge function.

	bridge := reflect.MakeFunc(target, func(args []reflect.Value) []reflect.Value {

		goArgs := make([]any, len(args))

		for i, arg := range args {
			goArgs[i] = arg.Interface()
		}

		result, cerr := invoker.CallStarlark(callable, goArgs, nil)
		if cerr != nil {
			return funcError(target, numValues, hasError, cerr)
		}

		return funcReturn(target, numValues, hasError, result)
	})

	return bridge.Interface(), nil
}

// Init loads the compiled program, executes its toplevel, and returns the named function as a callable.
//
// Fast path: if [Resource.Compiled] is non-empty and [Resource.CompilerVersion] matches the runtime's
// [starlark.CompilerVersion], the program loads directly from the in-memory cache. This is the common case within
// a process.
//
// Fallback path: opens the pack from RecoverySite via mmap, inspects the header, and either (a) loads bytecode from
// the compiled section when the compiler version matches, or (b) reads the source section, recompiles, and caches
// the new bytecode on the Resource. Both sub-paths refresh the in-memory Compiled cache so subsequent Init calls in
// the same process stay on the fast path.
//
// Parameters:
//   - `thread`: the starlark thread for program initialization.
//
// Returns:
//   - `starlark.Callable`: the live function.
//   - `error`: non-nil if loading, compiling, or initialization fails.
func (f *Resource) Init(thread *starlark.Thread) (starlark.Callable, error) {

	prog, err := f.loadProgram()
	if err != nil {
		return nil, fmt.Errorf("function.Resource init: %w", err)
	}

	globals, err := prog.Init(thread, nil)
	if err != nil {
		return nil, fmt.Errorf("function.Resource init: %w", err)
	}

	fn, ok := globals[f.FuncName]
	if !ok {
		return nil, fmt.Errorf("function.Resource init: function %q not found", f.FuncName)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("function.Resource init: %q is %s, not callable", f.FuncName, fn.Type())
	}

	return callable, nil
}

// Pack implements [op.Packer], overriding the embedded [mem.Resource] implementation.
//
// The transportable content is a [transportEnvelope]: the callable's metadata (FuncName, ParamNames, OriginalPos)
// plus the synthesized source read back from the archived pack. The raw pack file would be the wrong payload — it
// carries bytecode (host-specific, recompiled on the target) and no function name, and its digest is not the
// identity (the URI's SHA-256 covers the source bytes alone). Fixed-field JSON keeps the envelope deterministic, so
// pack → unpack → pack round-trips byte-identical.
//
// Returns:
//   - `[]byte`: the JSON-encoded [transportEnvelope].
//   - `error`: missing pack (a URI-only rehydrated resource with no local archive), or a read failure.
func (f *Resource) Pack() ([]byte, error) {

	source, err := f.sourceBytes()
	if err != nil {
		return nil, fmt.Errorf("function.Resource: pack %s: %w", f.URI(), err)
	}

	return json.Marshal(transportEnvelope{
		FuncName:    f.FuncName,
		ParamNames:  f.ParamNames,
		OriginalPos: f.OriginalPos,
		Source:      source,
	})
}

// Unpack implements [op.Unpacker], overriding the embedded [mem.Resource] implementation.
//
// Decodes the [transportEnvelope] and rebuilds the resource from its source text via [newFromSource] — compiling
// fresh bytecode for this host and archiving the pack into the local content-addressed store. The receiver carries
// no state (graph load dispatches Unpack on a zero value resolved from the URI fragment's type id). The rebuilt URI
// must equal `uri`: identity is the SHA-256 of the source bytes, covered by the graph checksum and signature through
// the slot URIs, so the equality check is what catches tampered content.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment; supplies the store root the pack materializes into.
//   - `uri`: the canonical tag URI recorded in the document.
//   - `content`: the JSON-encoded [transportEnvelope] produced by [Resource.Pack].
//
// Returns:
//   - `op.Resource`: the reconstructed *function.Resource, not interned in any catalog.
//   - `error`: envelope decode failure, compilation or store write failure, or a URI mismatch (integrity failure).
func (f *Resource) Unpack(runtimeEnvironment *op.RuntimeEnvironment, uri string, content []byte) (op.Resource, error) {

	var envelope transportEnvelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, fmt.Errorf("function.Resource: unpack %s: decode envelope: %w", uri, err)
	}

	candidate, err := newFromSource(
		runtimeEnvironment, envelope.FuncName, envelope.ParamNames, envelope.OriginalPos, envelope.Source)
	if err != nil {
		return nil, fmt.Errorf("function.Resource: unpack %s: %w", uri, err)
	}

	if candidate.URI() != uri {
		return nil, fmt.Errorf(
			"function.Resource: unpack: source digests to %s, document records %s (content altered)",
			candidate.URI(), uri)
	}

	return candidate, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// loadProgram returns a compiled [starlark.Program] for this Resource.
//
// Checks the in-memory Compiled cache first. On miss or version mismatch, opens the pack from RecoverySite via
// mmap, reads the header, and either loads bytecode from the compiled section (version match) or recompiles from
// the source section (version mismatch or no compiled payload). Refreshes the in-memory cache on every
// fallback-path success so subsequent calls hit the fast path.
//
// Returns:
//   - `*starlark.Program`: the compiled program.
//   - `error`: cache load, mmap, header parse, read, or compile failure.
func (f *Resource) loadProgram() (*starlark.Program, error) {

	// Fast path: cached in-memory bytecode matches current compiler version.
	if len(f.Compiled) > 0 && f.CompilerVersion == starlark.CompilerVersion {
		prog, err := starlark.CompiledProgram(bytes.NewReader(f.Compiled))
		if err != nil {
			return nil, fmt.Errorf("load cached bytecode: %w", err)
		}
		return prog, nil
	}

	// Fallback: open the pack from the URI-derived SourcePath.

	abs := f.SourcePath().Abs()
	if abs == "" {
		return nil, fmt.Errorf("no SourcePath — Resource was not archived")
	}

	m, err := mmap.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("mmap %s: %w", abs, err)
	}
	//nolint:errcheck // diagnose-ignored-error: module close; see docs/architecture/2.8-eventing-infrastructure.md
	defer func() { _ = m.Close() }()

	h, err := readFunctionPackHeader(m, int64(m.Len()))
	if err != nil {
		return nil, err
	}

	if h.CompiledSize > 0 && h.CompilerVersion == starlark.CompilerVersion {

		compiledBytes := make([]byte, h.CompiledSize)
		//nolint:gosec // G115: bounded by readFunctionPackHeader's section validation against the pack size.
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

// sourceBytes reads the synthesized source text back out of the archived pack.
//
// Opens the pack at the URI-derived SourcePath via mmap and returns the source section — the bytes whose SHA-256 is
// the resource's identity. [Resource.Pack] uses it to build the transport envelope.
//
// Returns:
//   - `[]byte`: the source text.
//   - `error`: missing SourcePath (the Resource was not archived locally), mmap, header parse, or read failure.
func (f *Resource) sourceBytes() ([]byte, error) {

	abs := f.SourcePath().Abs()
	if abs == "" {
		return nil, fmt.Errorf("no SourcePath — Resource was not archived")
	}

	m, err := mmap.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("mmap %s: %w", abs, err)
	}
	//nolint:errcheck // diagnose-ignored-error: module close; see docs/architecture/2.8-eventing-infrastructure.md
	defer func() { _ = m.Close() }()

	h, err := readFunctionPackHeader(m, int64(m.Len()))
	if err != nil {
		return nil, err
	}

	return io.ReadAll(sourceReader(m, h))
}

// endregion

// endregion

// region HELPER FUNCTIONS

// compileSource parses and compiles the given starlark source text.
//
// Parameters:
//   - `source`: the UTF-8 source text of a self-contained synthetic file.
//
// Returns:
//   - `*starlark.Program`: the compiled program, ready for Init.
//   - `error`: any parse or compile error.
func compileSource(source []byte) (*starlark.Program, error) {

	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, "<function>", source, func(string) bool { return false },
	)
	return prog, err
}

// programToBytes serializes a compiled program via [starlark.Program.Write].
//
// Parameters:
//   - `prog`: the compiled program.
//
// Returns:
//   - `[]byte`: the serialized bytecode, suitable for [starlark.CompiledProgram].
//   - `error`: any error from Program.Write.
func programToBytes(prog *starlark.Program) (_ []byte, err error) {

	var buf bytes.Buffer

	if err := prog.Write(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// funcError builds the return slice for a failed starlark call.
//
// If the Go func has an error return, the error occupies the last position and value slots are zeroed. If the Go
// func has no error return, funcError panics — the caller chose a signature that cannot report errors.
//
// Parameters:
//   - `target`: the Go func type the bridge implements.
//   - `numValues`: number of non-error return values.
//   - `hasError`: true when the last return slot is `error`.
//   - `err`: the error to surface.
//
// Returns:
//   - `[]reflect.Value`: zero values for non-error slots, and the error in the error slot. Panics when hasError is
//     false.
func funcError(target reflect.Type, numValues int, hasError bool, err error) []reflect.Value {

	assert.Truef(hasError, "starlark bridge: %v", err)

	out := make([]reflect.Value, target.NumOut())

	for i := range numValues {
		out[i] = reflect.Zero(target.Out(i))
	}

	out[len(out)-1] = reflect.ValueOf(&err).Elem()

	return out
}

// funcReturn builds the return slice for a successful starlark call.
//
// Parameters:
//   - `target`: the Go func type the bridge implements.
//   - `numValues`: number of non-error return values.
//   - `hasError`: true when the last return slot is `error`.
//   - `result`: the call's result as a native Go value.
//
// Returns:
//   - `[]reflect.Value`: the result converted to the Go return type, with nil error in the error slot when present.
func funcReturn(target reflect.Type, numValues int, hasError bool, result any) []reflect.Value {

	out := make([]reflect.Value, target.NumOut())

	if numValues == 1 {
		out[0] = reflect.ValueOf(result).Convert(target.Out(0))
	}

	if hasError {
		out[len(out)-1] = reflect.Zero(target.Out(len(out) - 1))
	}

	return out
}

// goToStarlark converts a [reflect.Value] to a [starlark.Value].
//
// Parameters:
//   - `rv`: the Go value to convert.
//
// Returns:
//   - `starlark.Value`: the converted Starlark value.
//   - `error`: non-nil if the type is unsupported.
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
//   - `sv`: the Starlark value to convert.
//
// Returns:
//   - `any`: the native Go value.
//   - `error`: non-nil if the type is unsupported.
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

// region SUPPORTING TYPES

// transportEnvelope is the transport form of a function resource — the payload [Resource.Pack] writes into a graph
// document's content section and [Resource.Unpack] reads back.
//
// It carries the callable's metadata alongside the synthesized source: the source alone cannot rebuild the resource
// because [Resource.Init] selects the callable from the executed module's globals by FuncName. Bytecode deliberately
// does not travel — the target host recompiles from source ([newFromSource]), which also absorbs compiler-version
// differences. Fixed field order keeps the JSON encoding deterministic.
type transportEnvelope struct {

	// FuncName is the function's name in the synthetic file (or the anonymous placeholder).
	FuncName string `json:"func_name"`

	// ParamNames is the ordered list of parameter names.
	ParamNames []string `json:"param_names,omitempty"`

	// OriginalPos is the original `.star` source position, or "" when unknown.
	OriginalPos string `json:"original_pos,omitempty"`

	// Source is the self-contained synthetic source text; its SHA-256 is the resource's identity.
	Source []byte `json:"source"`
}

// endregion
