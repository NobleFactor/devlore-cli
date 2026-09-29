// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package starlint_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/lint/starlint"

	// The registry is populated by the generated gen packages' init functions, and these two inventories
	// are what collect them. Without both, every resolution below answers "unknown" and the tests pass
	// vacuously -- which is the failure mode worth naming, so TestRegistryIsPopulated guards it.
	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// TestRegistryIsPopulated asserts the resolver saw a real surface.
//
// Every other test here asks whether a name resolves. An empty registry answers "no" to all of them and
// the suite goes green having checked nothing, so this is the test that makes the rest mean something.
func TestRegistryIsPopulated(t *testing.T) {

	resolver := starlint.NewPhaseScriptResolver()

	// The graph namespace carried 28 providers and `plan` 25 attributes when this was written. The
	// assertion is a floor, not the count: a new provider must not fail this, an unlinked registry must.
	if got := len(resolver.Namespaces()); got < 20 {
		t.Errorf("graph namespace holds %d providers, want at least 20: %v", got, resolver.Namespaces())
	}

	if got := len(resolver.Bare()); got < 20 {
		t.Errorf("plan holds %d bare attributes, want at least 20: %v", got, resolver.Bare())
	}

	// The namespaces the docker fixture's defects turn on. A present namespace answers UnknownMethod for a
	// name it does not have; an absent one answers UnknownNamespace, which is how this tells them apart.
	for _, required := range []string{"file", "pkg", "service", "ui"} {
		if resolution, _ := resolver.Resolve(required, "no_such_method"); resolution != starlint.UnknownMethod {
			t.Errorf("namespace %q is absent: plan.%s.no_such_method resolved as %s", required, required, resolution)
		}
	}
}

// TestResolve covers every distinct resolution against the live registry.
//
// The `unknown-*` cases are the docker package's real defects, which shipped and could not execute on any
// platform (#721). The `resolved` cases are what those calls should have been, so a regression that broke
// resolution outright would fail here rather than looking like a clean run.
func TestResolve(t *testing.T) {

	resolver := starlint.NewPhaseScriptResolver()

	for _, testCase := range []struct {
		name      string
		namespace string
		method    string
		want      starlint.Resolution
	}{
		// The fixture's live defects.
		{"no package namespace", "package", "install", starlint.UnknownNamespace},
		{"no user provider", "user", "add_to_group", starlint.UnknownNamespace},
		{"no verify on plan", "", "verify", starlint.UnknownMethod},
		{"no notify on plan", "", "notify", starlint.UnknownMethod},
		{"no download on plan", "", "download", starlint.UnknownMethod},
		{"file has no write", "file", "write", starlint.UnknownMethod},

		// What those calls should have been.
		{"pkg install", "pkg", "install", starlint.Resolved},
		{"file write_text", "file", "write_text", starlint.Resolved},

		// A promoted provider surfaces directly on plan: note() comes from ui.
		{"promoted note", "", "note", starlint.Resolved},

		// Withheld by lore's policy. Both exist in the registry; neither may be called from a phase script.
		{"run is denied", "", "run", starlint.Denied},
		{"clear is denied", "", "clear", starlint.Denied},

		// Withheld by hermeticity: these make no deterministic claim, and a plan must produce the same
		// graph on any machine.
		{"load_definition is not deterministic", "", "load_definition", starlint.NotDeterministic},
		{"save_definition is not deterministic", "", "save_definition", starlint.NotDeterministic},
		{"spec is not deterministic", "", "spec", starlint.NotDeterministic},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			resolution, method := resolver.Resolve(testCase.namespace, testCase.method)

			if resolution != testCase.want {
				t.Errorf("plan.%s.%s resolved %s, want %s", testCase.namespace, testCase.method, resolution, testCase.want)
			}

			// A withheld method was found before it was withheld, so the finding can name it. An unknown
			// one was not found at all.
			switch testCase.want {
			case starlint.Resolved, starlint.Denied, starlint.NotDeterministic:
				if method == nil {
					t.Errorf("plan.%s.%s resolved %s with no method", testCase.namespace, testCase.method, resolution)
				}
			case starlint.UnknownNamespace, starlint.UnknownMethod:
				if method != nil {
					t.Errorf("plan.%s.%s resolved %s but returned method %s", testCase.namespace, testCase.method, resolution, method.Name())
				}
			}
		})
	}
}

// TestAcceptsKeyword covers keyword checking, including the case a list of parameter names cannot express.
//
// `plan.file.write(path=..., content=...)` was one of the shipped defects, and the correction is not only
// the method name: the parameter is `destination_path`, not `path`. A checker that resolved the method and
// stopped would have passed `write_text(path=...)`.
func TestAcceptsKeyword(t *testing.T) {

	resolver := starlint.NewPhaseScriptResolver()

	_, writeText := resolver.Resolve("file", "write_text")
	if writeText == nil {
		t.Fatal("plan.file.write_text did not resolve")
	}

	for _, testCase := range []struct {
		keyword string
		want    bool
	}{
		{"destination_path", true},
		{"content", true},
		{"mode", true},
		{"path", false}, // the shipped mistake
		{"contents", false},
	} {
		if got := resolver.AcceptsKeyword(writeText, testCase.keyword); got != testCase.want {
			t.Errorf("write_text accepts %q = %t, want %t", testCase.keyword, got, testCase.want)
		}
	}

	// A method declaring **kwargs accepts any keyword. This is why the resolver holds op.Method rather
	// than a list of parameter names -- a list says "kwargs" and cannot say "therefore anything".
	_, gather := resolver.Resolve("", "gather")
	if gather == nil {
		t.Fatal("plan.gather did not resolve")
	}

	if !resolver.AcceptsKeyword(gather, "any_keyword_at_all") {
		t.Error("gather declares **kwargs but rejected an arbitrary keyword")
	}

	if resolver.AcceptsKeyword(nil, "anything") {
		t.Error("a nil method accepted a keyword")
	}
}

// TestDeniedMatchesLore asserts the resolver withholds exactly what lore withholds.
//
// The point of sharing lorepackage.LifecycleVerbs is that a verb cannot be denied at run time and accepted
// by the linter. This is the test that would fail if someone reintroduced a second copy of the list.
func TestDeniedMatchesLore(t *testing.T) {

	resolver := starlint.NewPhaseScriptResolver()

	want := map[string]bool{"assemble": true, "clear": true, "load": true, "run": true, "save": true}

	got := resolver.Denied()

	if len(got) != len(want) {
		t.Fatalf("denied set is %v, want %d entries", got, len(want))
	}

	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected denied attribute %q", name)
		}
	}
}

// TestResolutionNames pins the rule names, which findings carry and the fixture inventory records.
//
// A resolution's String is a contract with testdata/docker-package.want.tsv. Renaming one silently would
// leave that file describing rules nothing produces.
func TestResolutionNames(t *testing.T) {

	for resolution, want := range map[starlint.Resolution]string{
		starlint.Resolved:         "resolved",
		starlint.UnknownNamespace: "unknown-namespace",
		starlint.UnknownMethod:    "unknown-method",
		starlint.Denied:           "denied",
		starlint.NotDeterministic: "not-deterministic",
	} {
		if got := resolution.String(); got != want {
			t.Errorf("resolution %d is %q, want %q", int(resolution), got, want)
		}
	}
}
