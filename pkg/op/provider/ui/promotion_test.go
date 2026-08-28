// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen" // announces the provider
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
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
func TestUIPromoted_ReplacesTheOutputBuiltins(t *testing.T) {

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

// TestUIAtRoot_PrintReachesTheNarrator closes the gap the registration test leaves open.
//
// [TestUIAtRoot_ReplacesTheOutputBuiltins] proves the six names are INSTALLED, which — given that starlark's
// resolver checks predeclared before universal — means a script's print resolves here. It does not prove the
// bytes arrive anywhere, and the whole reason for taking the root namespace is where the bytes go: the builtin
// writes straight to stderr through starlark-go, escaping --silent, color, program-name prefixing, and the
// diagnostics stream of 2.8-eventing-infrastructure.md.
//
// So this executes a script that calls the bare name and asserts the narrator saw it. Registration and
// emission are separate claims, and only this one is about the bytes.
func TestUIPromoted_PrintReachesTheNarrator(t *testing.T) {

	receiverType, found := op.ReceiverRegistry().Type("ui")
	if !found {
		t.Fatal("the ui provider is not in the registry")
	}

	provider, isProvider := receiverType.(op.ProviderReceiverType)
	if !isProvider {
		t.Fatalf("ui registered as %T, want op.ProviderReceiverType", receiverType)
	}

	capture, buffer := sink.Capture()

	environment := &op.RuntimeEnvironment{
		Modules: []op.ProviderReceiverType{provider},
		Status:  status.NewNarrator("test", capture),
	}

	root := t.TempDir()
	const script = "print_reaches_narrator.star"

	//nolint:gosec // diagnose-ignored-error: test fixture written to t.TempDir(), not a user-supplied path
	if err := os.WriteFile(filepath.Join(root, script), []byte(`print("bytes from a bare print")`+"\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	if _, err := starlarkbridge.NewRuntime(environment).Invoke(script, root); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if got := buffer.String(); !strings.Contains(got, "bytes from a bare print") {
		t.Errorf("narrator captured %q; a bare print(...) must reach the narrator, not stderr", got)
	}
}
