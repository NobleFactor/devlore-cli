// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package claimcheck_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/claimcheck"
)

// TestEveryClaimHolds is the gate: a `+devlore:claim=` directive that its own code contradicts fails the build.
//
// It loads the whole module, not just the providers: claims live on provider methods, but a claiming method may
// call an in-module helper, and the helper's body has to be readable for propagation to follow it. One load
// covers everything — loading runs the compiler front end, which is the point of doing this once here rather
// than once per provider inside codegen.
func TestEveryClaimHolds(t *testing.T) {

	// Every platform CI builds, not just the developer's. Build tags select different bodies, so a claim can
	// hold here and fail on the machine that ships it — and the host platform is the one nobody checks, because
	// it is the one that always passed.
	for _, goos := range []string{"", "darwin", "linux", "windows"} {

		label := goos
		if label == "" {
			label = "host"
		}

		t.Run(label, func(t *testing.T) {

			violations, err := claimcheck.CheckGOOS(goos, "github.com/NobleFactor/devlore-cli/...")
			if err != nil {
				t.Fatalf("claimcheck.CheckGOOS(%q): %v", goos, err)
			}

			for _, violation := range violations {
				t.Error(violation)
			}
		})
	}
}

// TestCheck_CatchesEveryViolationShape proves the checker can fail, and on the shapes that matter.
//
// TestEveryClaimHolds passing means no claim in the tree is false. It does not mean the checker works — a check
// that reported nothing would pass it identically. These assertions are the other half.
//
// The fixture lives under testdata, which `./...` excludes, so its deliberately false claims never reach the
// registry or the conformance run.
func TestCheck_CatchesEveryViolationShape(t *testing.T) {

	violations, err := claimcheck.Check("./testdata/violator")
	if err != nil {
		t.Fatalf("claimcheck.Check: %v", err)
	}

	byMethod := map[string]string{}
	for _, violation := range violations {
		byMethod[violation.Method] = violation.String()
	}

	for _, expected := range []struct{ method, why string }{
		{"violator.DirectCall", "a capability call in the claiming body"},
		{"violator.FunctionValue", "a capability stored as a value, which a call-only scan misses (#683's shape)"},
		{"violator.ThroughHelper", "a capability reached only through a hop, which a body-only check misses"},
		{"violator.UnsandboxedRead", "unbounded I/O under a sandboxed claim"},
	} {
		if _, caught := byMethod[expected.method]; !caught {
			t.Errorf("%s went unreported; it is %s", expected.method, expected.why)
		}
	}

	// The guard that earns the type checker its keep: os.FileMode is a type, os.ModePerm a constant, and
	// os.FileMode(0o644) a conversion that parses as a call. Reporting any of them would fail ten real methods.
	if reported, caught := byMethod["violator.TypesAndConstantsOnly"]; caught {
		t.Errorf("types and constants must not count as reaching a capability, but got: %s", reported)
	}
}
