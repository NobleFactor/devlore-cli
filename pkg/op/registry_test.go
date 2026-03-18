// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sort"
	"testing"
)

// registryTestAction is a minimal Action implementation for testing the registry.
type registryTestAction struct {
	name string
}

func (a *registryTestAction) Name() string        { return a.name }
func (a *registryTestAction) Params() []ParamInfo { return nil }
func (a *registryTestAction) Do(_ *Context, _ map[string]any) (Result, Complement, error) {
	return nil, nil, nil
}

func TestNewActionRegistry(t *testing.T) {
	reg := NewActionRegistry()
	if reg == nil {
		t.Fatal("NewActionRegistry() returned nil")
	}
	if len(reg.actions) != 0 {
		t.Errorf("new registry has %d actions, want 0", len(reg.actions))
	}
}

func TestRegisterAndGet(t *testing.T) {
	reg := NewActionRegistry()
	action := &registryTestAction{name: "file.link"}
	reg.Register(action)

	got, ok := reg.Get("file.link")
	if !ok {
		t.Fatal("Get(file.link) returned false")
	}
	if got.Name() != "file.link" {
		t.Errorf("Get(file.link).ReceiverName() = %q, want %q", got.Name(), "file.link")
	}
}

func TestGet_Missing(t *testing.T) {
	reg := NewActionRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) returned true, want false")
	}
}

func TestMustGet_Existing(t *testing.T) {
	reg := NewActionRegistry()
	action := &registryTestAction{name: "shell.run"}
	reg.Register(action)

	got := reg.MustGet("shell.run")
	if got.Name() != "shell.run" {
		t.Errorf("MustGet(shell.run).ReceiverName() = %q, want %q", got.Name(), "shell.run")
	}
}

func TestMustGet_Missing_Panics(t *testing.T) {
	reg := NewActionRegistry()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustGet(missing) did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		want := "unregistered action: missing"
		if msg != want {
			t.Errorf("panic message = %q, want %q", msg, want)
		}
	}()
	reg.MustGet("missing")
}

func TestNames(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&registryTestAction{name: "file.link"})
	reg.Register(&registryTestAction{name: "shell.run"})
	reg.Register(&registryTestAction{name: "template.render_bytes"})

	got := reg.Names()
	sort.Strings(got)
	want := []string{"file.link", "shell.run", "template.render_bytes"}
	if len(got) != len(want) {
		t.Fatalf("Names() returned %d items, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestNames_Empty(t *testing.T) {
	reg := NewActionRegistry()
	got := reg.Names()
	if len(got) != 0 {
		t.Errorf("Names() on empty registry = %v, want []", got)
	}
}

func TestRegister_Overwrites(t *testing.T) {
	reg := NewActionRegistry()
	original := &registryTestAction{name: "file.link"}
	replacement := &registryTestAction{name: "file.link"}
	reg.Register(original)
	reg.Register(replacement)

	got, ok := reg.Get("file.link")
	if !ok {
		t.Fatal("Get(file.link) returned false after overwrite")
	}
	// Verify the replacement is what we get back (same pointer)
	if got != replacement {
		t.Error("Get(file.link) did not return the replacement action")
	}
}

func TestRegister_Multiple(t *testing.T) {
	reg := NewActionRegistry()
	names := []string{"a.one", "b.two", "c.three", "d.four"}
	for _, name := range names {
		reg.Register(&registryTestAction{name: name})
	}
	for _, name := range names {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("Get(%q) returned false after registering multiple", name)
		}
	}
	if len(reg.Names()) != len(names) {
		t.Errorf("Names() returned %d, want %d", len(reg.Names()), len(names))
	}
}
