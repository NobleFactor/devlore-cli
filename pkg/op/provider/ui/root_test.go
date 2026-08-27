// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package ui_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen" // announces the provider
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
)

// TestUIAtRoot_ReplacesTheOutputBuiltins pins the property the root placement exists for, and which is
// otherwise invisible.
//
// A script calling print("x") looks identical whether it reaches this provider or starlark's builtin, and the
// difference is where the bytes go: the builtin writes straight to stderr through starlark-go, escaping
// --silent, color, program-name prefixing, and the diagnostics stream of 2.8-eventing-infrastructure.md.
// Nothing about the call site reveals which one ran, so without this test a regression that dropped
// +devlore:root=true would restore the builtin silently and every script would keep working.
//
// The assertion is the mechanism rather than a proxy for it. Starlark's resolver checks predeclared names
// BEFORE universal ones (resolve.go), so a predeclared "print" IS the replacement -- there is no separate act
// of overriding to observe.
func TestUIAtRoot_ReplacesTheOutputBuiltins(t *testing.T) {

	receiverType, found := op.ReceiverRegistry().Type("ui")
	if !found {
		t.Fatal(`the ui provider is not in the registry; the gen blank import should have announced it`)
	}

	provider, isProvider := receiverType.(op.ProviderReceiverType)
	if !isProvider {
		t.Fatalf("ui registered as %T, want op.ProviderReceiverType", receiverType)
	}

	environment := &op.RuntimeEnvironment{Modules: []op.ProviderReceiverType{provider}}
	predeclared := starlarkbridge.NewRuntime(environment).Predeclared()

	// print and fail are starlark's; note, warn, error, and succeed are ours alone. Both kinds must land.
	for _, name := range []string{"error", "fail", "note", "print", "succeed", "warn"} {
		if _, present := predeclared[name]; !present {
			t.Errorf("predeclared lacks %q; a root-placed provider installs each method as a top-level global", name)
		}
	}

	// Root placement REPLACES the provider global rather than adding to it, which is why every ui.* call site
	// in the tree had to move. If this ever passes, the sweep was undone and both spellings work again.
	if _, present := predeclared["ui"]; present {
		t.Error(`predeclared contains "ui"; a root-placed provider exposes its methods, not itself`)
	}
}
