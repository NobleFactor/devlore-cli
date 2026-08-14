// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// TestVariableByName_ResolvesSourceSetAfterRegistration verifies the cache-on-found (sync.OnceValues) semantics.
//
// A parameter registered before its source is supplied still resolves once the source is set. VariableByName
// resolves lazily on first read, so the application need not order source population before provider construction —
// the ordering bug that hid the "config" override behind cfg.Load() and made cfg.lint unresolvable.
func TestVariableByName_ResolvesSourceSetAfterRegistration(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	// Register the parameter BEFORE any source supplies it (mirrors provider construction at NewRuntime).
	if err := runtimeEnvironment.RegisterParameter(Parameter{Name: "late", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("RegisterParameter: %v", err)
	}

	// The application supplies the value AFTER registration (mirrors app.Overrides["config"] set after the runtime
	// is built). No read happens before this point, so the lazy resolver has not cached an absence.
	app.Overrides = map[string]any{"late": "set-after-registration"}

	// First read resolves it lazily and caches.
	got, ok := runtimeEnvironment.VariableByName("late")
	if !ok || got.Value != "set-after-registration" {
		t.Fatalf("VariableByName(late) = (%v, %v), want (%q, true)", got.Value, ok, "set-after-registration")
	}

	// Subsequent reads return the cached value.
	if again, ok := runtimeEnvironment.VariableByName("late"); !ok || again.Value != "set-after-registration" {
		t.Errorf("second VariableByName(late) = (%v, %v), want the cached value", again.Value, ok)
	}
}

// TestVariableByName_ReadBeforeSourceSet pins the sharp edge of the sync.OnceValues semantic.
//
// Because the first read is memoized, reading a variable BEFORE its source is set caches the absence permanently —
// a later set is not picked up. This is acceptable only because no provider reads a variable at construction (the
// first read always follows source population). The test documents the contract, so a future read-before-set is a
// deliberate choice and not a silent regression of the config-resolution fix.
func TestVariableByName_ReadBeforeSourceSet(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	if err := runtimeEnvironment.RegisterParameter(Parameter{Name: "early", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("RegisterParameter: %v", err)
	}

	// Read BEFORE the source is set — resolves absent, and OnceValues memoizes that result.
	if _, ok := runtimeEnvironment.VariableByName("early"); ok {
		t.Fatal("expected 'early' unresolved before its source is set")
	}

	// Set the source AFTER the first read.
	app.Overrides = map[string]any{"early": "too-late"}

	// The memoized absence is NOT overturned — documents the compute-once trade-off.
	if v, ok := runtimeEnvironment.VariableByName("early"); ok {
		t.Errorf("VariableByName(early) = (%v, %v); want still-absent (read-before-set is memoized)", v.Value, ok)
	}
}

// TestNewRuntimeEnvironment_MintsRootFromAnchor verifies the option-4 contract (issue #393).
//
// The spec carries only the anchor path and mode, and the environment mints — and, via Close, releases — its own
// confined Root.
func TestNewRuntimeEnvironment_MintsRootFromAnchor(t *testing.T) {

	dir := t.TempDir()
	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app).WithRoot(dir, fsroot.ModeConfined)

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	if runtimeEnvironment.Root() == nil {
		t.Fatal("Root = nil, want a minted confined root")
	}
	if got := runtimeEnvironment.Root().Name(); got != dir {
		t.Errorf("Root.Name() = %q, want %q", got, dir)
	}
	if runtimeEnvironment.RecoverySite == nil {
		t.Error("RecoverySite = nil, want wired (a root was minted)")
	}
	if err := runtimeEnvironment.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestNewRuntimeEnvironment_BadAnchorFails verifies a confined mint failure surfaces as the constructor's error.
//
// The error is the authoritative validity gate for every consumer (the executor turns it into a preflight failure).
func TestNewRuntimeEnvironment_BadAnchorFails(t *testing.T) {

	app := &application.Application{Name: "test"}
	missing := filepath.Join(t.TempDir(), "absent")
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app).WithRoot(missing, fsroot.ModeConfined)

	if _, err := NewRuntimeEnvironment(context.Background(), spec); err == nil {
		t.Fatal("NewRuntimeEnvironment = nil error, want a mint failure for the missing anchor")
	}
}

// TestNewRuntimeEnvironment_EmptyAnchorMeansNoRoot verifies the no-root semantics.
//
// An unset anchor is legal and advertised — `cmd/lore` builds such a session in production — and it yields no
// RecoverySite. HasRoot reports the state, and Root panics rather than handing back a nil interface that would
// fail later, inside fsroot, with a stack that never names the mistake.
func TestNewRuntimeEnvironment_EmptyAnchorMeansNoRoot(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	if runtimeEnvironment.HasRoot() {
		t.Error("HasRoot = true, want false (no anchor set)")
	}
	if runtimeEnvironment.RecoverySite != nil {
		t.Error("RecoverySite non-nil, want nil (no root)")
	}

	defer func() {
		if recover() == nil {
			t.Error("Root on a rootless session did not panic; the nil interface would fail later, inside fsroot")
		}
	}()
	_ = runtimeEnvironment.Root()
}

// TestRuntimeEnvironment_ScratchIsAPrivateTreeRemovedOnClose pins the scratch contract: a directory of the
// session's own inside the OS temp directory, the same instance on every call, gone with its contents on Close.
func TestRuntimeEnvironment_ScratchIsAPrivateTreeRemovedOnClose(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	scratch := runtimeEnvironment.Scratch()
	if scratch == nil {
		t.Fatal("Scratch = nil; a session always has one")
	}
	if again := runtimeEnvironment.Scratch(); again != scratch {
		t.Error("Scratch returned a second instance; the session's scratch is one tree, minted once")
	}

	// A rootless session still has scratch — the two are independent, which is the point of having no HasScratch.
	if runtimeEnvironment.HasRoot() {
		t.Error("HasRoot = true, want false")
	}

	if inside := scratch.Name(); !strings.HasPrefix(inside, os.TempDir()) {
		t.Errorf("scratch anchored at %q, want a directory inside %q", inside, os.TempDir())
	}

	// 0700 is what carries the protected DACL on Windows; mode bits are a Unix subject.
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(scratch.Name())
		if statErr != nil {
			t.Fatalf("Stat(scratch): %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("scratch mode = %v, want 0700", got)
		}
	}

	tree := scratch.Name()
	if err := runtimeEnvironment.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("Stat after Close = %v, want the scratch tree removed", err)
	}
}
