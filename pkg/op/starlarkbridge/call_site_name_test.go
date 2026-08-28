// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package starlarkbridge

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region Call-site naming (#710)

// TestBuiltinName_FollowsPlacement pins the rule a dispatch error answers to.
//
// Where a call site exists, an error names what the author typed. A promoted provider's methods ARE the
// top-level globals, so its provider name is not a symbol any script can use — reporting `ui.print` sends an
// author toward `undefined: ui`, a second and unrelated error. A qualified provider is the opposite: its name
// is exactly what the author typed, and must stay.
//
// The two are asserted together because a fix that made every name bare would satisfy the first alone.
func TestBuiltinName_FollowsPlacement(t *testing.T) {

	t.Run("promoted provider reports the bare method name", func(t *testing.T) {

		// A promoted provider installs each method as its own predeclared entry, so the global IS the builtin.
		predeclared := NewRuntime(&op.RuntimeEnvironment{
			Modules: bridgeFixtureModules(t, "bridgeRootModuleFixtureA"),
		}).Predeclared()

		value, present := predeclared["greet"]
		if !present {
			t.Fatalf("predeclared lacks \"greet\"; got %v", predeclared.Keys())
		}

		builtin, isBuiltin := value.(*starlark.Builtin)
		if !isBuiltin {
			t.Fatalf("predeclared[greet] is %T, want *starlark.Builtin", value)
		}

		if got := builtin.Name(); got != "greet" {
			t.Errorf("builtin name = %q, want %q; a promoted provider's name is not a symbol a script can use",
				got, "greet")
		}
	})

	t.Run("qualified provider keeps its qualifier", func(t *testing.T) {

		receiver := requireGlobal(t,
			NewRuntime(&op.RuntimeEnvironment{
				Modules: bridgeFixtureModules(t, "bridgeModuleFixture"),
			}).Predeclared(), "bridgeModuleFixture")

		attr, err := receiver.Attr("ping")
		if err != nil {
			t.Fatalf("Attr(ping): %v", err)
		}

		builtin, isBuiltin := attr.(*starlark.Builtin)
		if !isBuiltin {
			t.Fatalf("Attr(ping) is %T, want *starlark.Builtin", attr)
		}

		if got := builtin.Name(); !strings.Contains(got, ".") {
			t.Errorf("builtin name = %q, want it qualified; a qualified provider's name IS what the author typed",
				got)
		}
	})
}

// endregion
