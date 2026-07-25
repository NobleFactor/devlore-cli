// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sync"
	"testing"
)

// region TEST FIXTURES

// mustRegister registers invocation under label, failing the test on a non-nil error.
func mustRegister(t *testing.T, r *InvocationRegistry, label string, invocation *Invocation) {
	t.Helper()
	if err := r.Register(label, invocation); err != nil {
		t.Fatalf("Register(%q): %v", label, err)
	}
}

// endregion

func TestInvocationRegistry_New_IsEmpty(t *testing.T) {
	r := NewInvocationRegistry()

	if got := r.All(); len(got) != 0 {
		t.Errorf("All() on new registry = %d entries, want 0", len(got))
	}
	if got := r.ByLabel("anything"); got != nil {
		t.Errorf("ByLabel on new registry = %v, want nil", got)
	}
}

func TestInvocationRegistry_Register_AppendsInCreationOrder(t *testing.T) {
	r := NewInvocationRegistry()
	first, second, third := &Invocation{}, &Invocation{}, &Invocation{}

	mustRegister(t, r, "first", first)
	mustRegister(t, r, "second", second)
	mustRegister(t, r, "third", third)

	got := r.All()
	want := []*Invocation{first, second, third}
	if len(got) != len(want) {
		t.Fatalf("All() = %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %p, want %p", i, got[i], want[i])
		}
	}
}

func TestInvocationRegistry_Register_IndexesByLabel(t *testing.T) {
	r := NewInvocationRegistry()
	invocation := &Invocation{}

	mustRegister(t, r, "file.copy#1", invocation)

	if got := r.ByLabel("file.copy#1"); got != invocation {
		t.Errorf("ByLabel = %p, want %p", got, invocation)
	}
}

func TestInvocationRegistry_Register_RejectsDuplicateLabel(t *testing.T) {
	r := NewInvocationRegistry()
	first := &Invocation{}
	mustRegister(t, r, "dup", first)

	if err := r.Register("dup", &Invocation{}); err == nil {
		t.Fatal("Register with duplicate label = nil error, want error")
	}

	// The rejected registration leaves both structures untouched.
	if got := r.All(); len(got) != 1 || got[0] != first {
		t.Errorf("All() after rejected duplicate = %v, want [first]", got)
	}
	if got := r.ByLabel("dup"); got != first {
		t.Errorf("ByLabel after rejected duplicate = %p, want first %p", got, first)
	}
}

func TestInvocationRegistry_ByLabel_ReturnsNilForUnknown(t *testing.T) {
	r := NewInvocationRegistry()
	mustRegister(t, r, "known", &Invocation{})

	if got := r.ByLabel("unknown"); got != nil {
		t.Errorf("ByLabel(unknown) = %v, want nil", got)
	}
}

func TestInvocationRegistry_AutoLabel_IncrementsPerProviderMethod(t *testing.T) {
	r := NewInvocationRegistry()

	if got := r.AutoLabel("file.write_text"); got != "file.write_text#1" {
		t.Errorf("AutoLabel #1 = %q, want file.write_text#1", got)
	}
	if got := r.AutoLabel("file.write_text"); got != "file.write_text#2" {
		t.Errorf("AutoLabel #2 = %q, want file.write_text#2", got)
	}

	// A different provider.method has its own independent counter.
	if got := r.AutoLabel("plan.choose"); got != "plan.choose#1" {
		t.Errorf("AutoLabel for new method = %q, want plan.choose#1", got)
	}
	if got := r.AutoLabel("file.write_text"); got != "file.write_text#3" {
		t.Errorf("AutoLabel resumes per-method = %q, want file.write_text#3", got)
	}
}

func TestInvocationRegistry_All_ReturnsIndependentCopy(t *testing.T) {
	r := NewInvocationRegistry()
	registered := &Invocation{}
	mustRegister(t, r, "a", registered)

	snapshot := r.All()
	snapshot[0] = &Invocation{} // mutating the returned slice must not reach the registry

	if got := r.All(); len(got) != 1 || got[0] != registered {
		t.Errorf("registry mutated through returned slice; All() = %v, want [registered]", got)
	}

	// A later registration must not grow a previously returned snapshot.
	mustRegister(t, r, "b", &Invocation{})
	if len(snapshot) != 1 {
		t.Errorf("prior snapshot length = %d after later registration, want 1", len(snapshot))
	}
}

func TestInvocationRegistry_Reset_ClearsEntriesAndCounters(t *testing.T) {
	r := NewInvocationRegistry()
	mustRegister(t, r, "file.copy#1", &Invocation{})
	r.AutoLabel("file.copy")

	r.Reset()

	if got := r.All(); len(got) != 0 {
		t.Errorf("All() after Reset = %d entries, want 0", len(got))
	}
	if got := r.ByLabel("file.copy#1"); got != nil {
		t.Errorf("ByLabel after Reset = %v, want nil", got)
	}
	// The auto-label counter restarts at 1.
	if got := r.AutoLabel("file.copy"); got != "file.copy#1" {
		t.Errorf("AutoLabel after Reset = %q, want file.copy#1", got)
	}
	// The label is freed, so the same label registers cleanly again.
	if err := r.Register("file.copy#1", &Invocation{}); err != nil {
		t.Errorf("Register after Reset reusing label = %v, want nil", err)
	}
}

func TestInvocationRegistry_Concurrent_IsRaceFree(t *testing.T) {
	r := NewInvocationRegistry()

	const goroutines = 8
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				// All goroutines contend on one provider.method counter; AutoLabel's
				// monotonic ordinal keeps every label unique, so every Register succeeds —
				// verified by the entry count after the run.
				label := r.AutoLabel("p.m")
				_ = r.Register(label, &Invocation{})
			}
		}()
	}
	wg.Wait()

	if got := len(r.All()); got != goroutines*perGoroutine {
		t.Errorf("All() after concurrent registration = %d, want %d", got, goroutines*perGoroutine)
	}
	if got := r.AutoLabel("p.m"); got != fmt.Sprintf("p.m#%d", goroutines*perGoroutine+1) {
		t.Errorf("AutoLabel after concurrent run = %q, want p.m#%d", got, goroutines*perGoroutine+1)
	}
}
