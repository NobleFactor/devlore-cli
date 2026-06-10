// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// TestSourceKey_StarlarkFunction verifies the function provider's resource announcement registers
// *starlark.Function as a byType source key.
//
// The gen package's init runs AnnounceResource with the source type, so a fresh ReceiverRegistry resolves the
// constructor from a bare *starlark.Function via ConstructorForSource — the path the planner uses to build a
// function.Resource from a Starlark function without naming the function provider.
func TestSourceKey_StarlarkFunction(t *testing.T) {

	registry := op.ReceiverRegistry()

	construct, ok := registry.ConstructorForSource(reflect.TypeFor[*starlark.Function]())
	if !ok {
		t.Fatal("ConstructorForSource(*starlark.Function): not registered — byType source key missing")
	}
	if construct == nil {
		t.Fatal("ConstructorForSource(*starlark.Function): nil constructor")
	}
}
