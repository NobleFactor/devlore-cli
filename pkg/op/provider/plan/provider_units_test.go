// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// --- Plan error paths (the Go door's rejections) ---

func TestPlan_InvalidActionName_NoDot(t *testing.T) {
	p := NewProvider(plannedEnvironmentAt(t, t.TempDir()))

	if _, err := p.Plan(op.ActionName("nodot"), nil, nil); err == nil {
		t.Fatal("Plan() with a dotless action name should error")
	} else if !strings.Contains(err.Error(), "no dot") {
		t.Errorf("error = %q, want it to mention %q", err, "no dot")
	}
}

func TestPlan_UnknownActionProvider(t *testing.T) {
	p := NewProvider(plannedEnvironmentAt(t, t.TempDir()))

	if _, err := p.Plan(op.ActionName("bogus.mkdir"), nil, nil); err == nil {
		t.Fatal("Plan() with an unregistered action provider should error")
	} else if !strings.Contains(err.Error(), "unknown action provider") {
		t.Errorf("error = %q, want it to mention %q", err, "unknown action provider")
	}
}

func TestPlan_UnknownMethod(t *testing.T) {
	p := NewProvider(plannedEnvironmentAt(t, t.TempDir()))

	if _, err := p.Plan(op.ActionName("file.no_such_method"), nil, nil); err == nil {
		t.Fatal("Plan() with an unknown method on a known provider should error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention %q", err, "not found")
	}
}

func TestPlan_MalformedKwargs_WrongType(t *testing.T) {
	p := NewProvider(plannedEnvironmentAt(t, t.TempDir()))

	// file.mkdir's `mode` parameter is an os.FileMode (numeric); a non-numeric string cannot convert to it.
	_, err := p.Plan(file.Mkdir, nil, map[string]any{
		"path": "/tmp/backfill",
		"mode": "not-a-file-mode",
		"user": "", "group": "",
	})
	if err == nil {
		t.Fatal("Plan() with a wrong-typed kwarg should error")
	}
}

// --- Origin ---

func TestOrigin_StampsScope(t *testing.T) {
	p := NewProvider(plannedEnvironmentAt(t, t.TempDir()))

	origin := p.Origin("system")
	if origin.Scope() != "system" {
		t.Errorf("Origin(%q).Scope() = %q, want %q", "system", origin.Scope(), "system")
	}
	// Tool is framework-owned; AssembleDefinition stamps it, not Origin.
	if origin.Tool() != "" {
		t.Errorf("Origin().Tool() = %q, want empty (stamped by AssembleDefinition)", origin.Tool())
	}
}

// --- Clear ---

func TestClear_ResetsLedger(t *testing.T) {
	tmp := t.TempDir()
	p := NewProvider(plannedEnvironmentAt(t, tmp))

	plannedMkdir(t, p, filepath.Join(tmp, "one"))
	plannedMkdir(t, p, filepath.Join(tmp, "two"))
	if got := len(p.InvocationRegistry().All()); got != 2 {
		t.Fatalf("before Clear: registry has %d invocations, want 2", got)
	}

	if err := p.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if got := len(p.InvocationRegistry().All()); got != 0 {
		t.Errorf("after Clear: registry has %d invocations, want 0", got)
	}
}
