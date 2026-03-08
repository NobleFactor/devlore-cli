// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// CallableResource is the interface that mem.Callable satisfies.
// It allows pkg/op to work with callables without importing the mem package.
type CallableResource interface {
	Resource
	Init(thread *starlark.Thread) error
	Fn() starlark.Callable
	FuncTypeName() string
}

// callableExtractorFn is the registered function that extracts a
// *starlark.Function into a CallableResource. Registered by the mem
// package in init(). Returns the extracted, compiled callable.
var callableExtractorFn func(fn *starlark.Function, funcType string) (CallableResource, error)

// RegisterCallableExtractor registers the function that extracts a
// *starlark.Function into a CallableResource. Called by the mem package
// during init().
func RegisterCallableExtractor(fn func(*starlark.Function, string) (CallableResource, error)) {
	callableExtractorFn = fn
}

// ExtractCallable extracts a *starlark.Function into a CallableResource
// using the registered extractor. Returns an error if no extractor is
// registered.
func ExtractCallable(fn *starlark.Function, funcType string) (CallableResource, error) {
	if callableExtractorFn == nil {
		return nil, fmt.Errorf("no callable extractor registered (mem package not imported?)")
	}
	return callableExtractorFn(fn, funcType)
}

// isCallableResource returns true if the value implements CallableResource.
func isCallableResource(v any) bool {
	_, ok := v.(CallableResource)
	return ok
}

// isFuncType returns true if the reflect.Type is a function type.
func isFuncType(t reflect.Type) bool {
	return t.Kind() == reflect.Func
}
