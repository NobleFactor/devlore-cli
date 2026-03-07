// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"go.starlark.net/starlark"
)

// Callable is a mem.Resource that holds a Starlark function extracted into
// a self-contained synthetic source file with compiled bytecode.
//
// The URI is opaque: mem:callable/<FuncType>/<Name>. FuncType is the named
// Go type the callable satisfies (e.g., "file.Reducer", "Predicate"). Name
// is the function name or <action>.<param> for lambdas.
//
// The Data field (inherited from Resource) holds the synthetic source text.
// The Compiled field holds serialized bytecode from Program.Write. Both are
// []byte — serializable, transferable, persistable.
//
// Lifecycle:
//  1. Extract(*starlark.Function) → Callable (Phase 2)
//  2. Compile() → populates Compiled bytecode (Phase 3)
//  3. Init(thread) → loads bytecode, extracts live fn (Phase 3)
//  4. Fn() → returns live callable for adapter invocation
type Callable struct {
	Resource // embeds mem.Resource (source text in Data)

	// Compiled bytecode — Program.Write output. Nil until Compile.
	Compiled []byte

	// URI identity fields — compose the opaque URI:
	// mem:callable/<FuncType>/<Name>
	FuncType string // named Go type: "file.Reducer", "Predicate"
	Name     string // function name or <action>.<param> for lambdas

	// Metadata captured at extraction time.
	FuncName        string   // function name in synthetic file ("_callable" or original)
	ParamNames      []string // parameter names (excluding swallowed)
	NumParams       int      // total params (for validation)
	CompilerVersion uint32   // starlark.CompilerVersion at compile time
	OriginalPos     string   // "recipe.star:42" (diagnostics only)

	// Live state — populated by Init(), not serialized.
	fn starlark.Callable
}

// NewCallable creates a Callable with the given function type and name.
// The source text (Data) and compiled bytecode should be set by the extraction
// and compilation phases.
func NewCallable(funcType, name string) *Callable {
	c := &Callable{
		Resource: NewResource("callable", funcType+"/"+name),
		FuncType: funcType,
		Name:     name,
		FuncName: "_callable",
	}
	// Override URI to use the callable-specific format.
	c.SetURI(c.callableURI())
	return c
}

// callableURI computes the opaque mem:callable URI.
func (c *Callable) callableURI() string {
	return "mem:callable/" + c.FuncType + "/" + c.Name
}

// SetSource sets the synthetic source text and recomputes the hash.
func (c *Callable) SetSource(source []byte) {
	c.Data = source
	c.ComputeHash()
}

// Fn returns the live callable. Panics if Init has not been called.
func (c *Callable) Fn() starlark.Callable {
	if c.fn == nil {
		panic("callable: Init not called")
	}
	return c.fn
}
