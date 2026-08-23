// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestResolveRecordedResource_HitReturnsTheCanonical pins the rearm's identity decode
// (4-resource-management.md §5.6): a reloaded producer result — the resource's URI string — resolves
// against the rehydrated catalog to the restored generation, never a fresh construction.
func TestResolveRecordedResource_HitReturnsTheCanonical(t *testing.T) {

	catalog := NewResourceCatalog()
	entry := newLifecycle("test:///produced", AddressingLocation)
	catalog.Resolve(entry)

	environment := &RuntimeEnvironment{ResourceCatalog: catalog}

	canonical, resolved := resolveRecordedResource(environment, "test:///produced", reflect.TypeFor[*lifecycleResource]())
	if !resolved {
		t.Fatal("resolveRecordedResource(hit) did not resolve")
	}
	if canonical != Resource(entry) {
		t.Errorf("resolved %p is not the catalog's canonical %p", canonical, entry)
	}
}

// TestResolveRecordedResource_MissAndNonStringFallThrough pins the rearm's documented tolerance: an
// unknown URI and a non-string result both fall through unresolved — the value is left as-is, and a
// consumer that needed the concrete type meets the dispatch seam's refusal at its own dispatch.
func TestResolveRecordedResource_MissAndNonStringFallThrough(t *testing.T) {

	environment := &RuntimeEnvironment{ResourceCatalog: NewResourceCatalog()}
	target := reflect.TypeFor[*lifecycleResource]()

	if _, resolved := resolveRecordedResource(environment, "test:///unknown", target); resolved {
		t.Error("a catalog miss must fall through unresolved (the rearm tolerates, dispatch refuses)")
	}
	if _, resolved := resolveRecordedResource(environment, 42, target); resolved {
		t.Error("a non-string result must fall through unresolved")
	}
	if _, resolved := resolveRecordedResource(environment, "test:///x", reflect.TypeFor[string]()); resolved {
		t.Error("a non-resource product type must fall through unresolved")
	}
}

func TestRecoveryStack_Unwind_LIFO(t *testing.T) {
	s := NewRecoveryStack()
	var order []int

	for i := range 3 {
		child := NewRecoveryStack()
		child.PushNested(tagStack(i, func(v int) { order = append(order, v) }))
		s.PushNested(child)
	}

	if err := s.Unwind(nil); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 compensations, got %d", len(order))
	}
	// LIFO: 2, 1, 0
	for i, want := range []int{2, 1, 0} {
		if order[i] != want {
			t.Errorf("order[%d] = %d, want %d", i, order[i], want)
		}
	}

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after Unwind", s.Len())
	}
}

func TestRecoveryStack_Unwind_BestEffort(t *testing.T) {
	s := NewRecoveryStack()
	var compensated []int

	// Entry 0: succeeds
	s.PushNested(tagStack(0, func(v int) { compensated = append(compensated, v) }))
	// Entry 1: fails
	s.PushNested(failStack(errors.New("compensate-1 failed")))
	// Entry 2: succeeds
	s.PushNested(tagStack(2, func(v int) { compensated = append(compensated, v) }))

	err := s.Unwind(nil)
	if err == nil {
		t.Fatal("Unwind() should return error when a compensation fails")
	}

	// Entry 1 failed, but entries 0 and 2 should still have been compensated (LIFO: 2, 0).
	if len(compensated) != 2 {
		t.Fatalf("expected 2 successful compensations, got %d: %v", len(compensated), compensated)
	}
	if compensated[0] != 2 || compensated[1] != 0 {
		t.Errorf("compensated = %v, want [2, 0]", compensated)
	}
}

func TestRecoveryStack_Unwind_RetainsJournalOnFailure(t *testing.T) {

	// A failed unwind must NOT wipe the stack: it is the compensation-failure journal (phase-8 step 21). A clean unwind
	// still clears it (nothing to journal).
	failing := NewRecoveryStack()
	failing.PushNested(failStack(errors.New("compensate failed")))
	if err := failing.Unwind(nil); err == nil {
		t.Fatal("Unwind() should return an error when a compensation fails")
	}
	if failing.Len() == 0 {
		t.Error("Len() = 0 after a failed unwind; the journal was destroyed (want it retained)")
	}

	clean := NewRecoveryStack()
	clean.PushNested(tagStack(0, func(int) {}))
	if err := clean.Unwind(nil); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}
	if clean.Len() != 0 {
		t.Errorf("Len() = %d after a clean unwind, want 0 (nothing dirty to journal)", clean.Len())
	}
}

func TestRecoveryStack_CompensationError_RoundTrips(t *testing.T) {

	// A receipt that failed to compensate carries its forward error (the source) and its compensation error (the
	// diagnostic); both must survive a trace save/load in either document format so a reloaded journal is faithful.
	receipt := &ReceiptBase{}
	if err := receipt.Commit(nil, nil, nil, errors.New("forward boom")); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	receipt.compensationError = errors.New("undo boom: resource stuck")

	stack := NewRecoveryStack()
	stack.Push(receipt)

	codecs := []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"json", json.Marshal, json.Unmarshal},
		{"yaml", yaml.Marshal, yaml.Unmarshal},
	}

	for _, codec := range codecs {
		t.Run(codec.name, func(t *testing.T) {

			data, err := codec.encode(stack)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			var reloaded RecoveryStack
			if err := codec.decode(data, &reloaded); err != nil {
				t.Fatalf("decode: %v", err)
			}

			receipts := reloaded.Receipts()
			if len(receipts) != 1 {
				t.Fatalf("reloaded Receipts() = %d, want 1", len(receipts))
			}
			if got := receipts[0].Err(); got == nil || got.Error() != "forward boom" {
				t.Errorf("reloaded Err() = %v, want %q (the source must survive)", got, "forward boom")
			}
			if got := receipts[0].CompensationError(); got == nil || got.Error() != "undo boom: resource stuck" {
				t.Errorf("reloaded CompensationError() = %v, want %q", got, "undo boom: resource stuck")
			}
		})
	}
}

func TestRecoveryStack_Discard(t *testing.T) {
	s := NewRecoveryStack()
	compensated := false

	s.PushNested(tagStack(0, func(int) { compensated = true }))

	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}

	s.Discard()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after Discard", s.Len())
	}
	if compensated {
		t.Error("compensate was called after Discard (should not unwind)")
	}
}

func TestRecoveryStack_Len(t *testing.T) {
	s := NewRecoveryStack()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0 for empty stack", s.Len())
	}

	s.PushNested(NewRecoveryStack())

	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}

	s.PushNested(NewRecoveryStack())

	if s.Len() != 2 {
		t.Errorf("Len() = %d, want 2", s.Len())
	}
}

func TestRecoveryStack_PushNested_AppendsOneEntry(t *testing.T) {
	parent := NewRecoveryStack()
	child := NewRecoveryStack()

	parent.PushNested(child)

	if parent.Len() != 1 {
		t.Errorf("parent.Len() = %d, want 1", parent.Len())
	}
}

func TestRecoveryStack_PushNested_NilPanics(t *testing.T) {
	parent := NewRecoveryStack()

	defer func() {
		if recover() == nil {
			t.Error("PushNested(nil) should panic (a nil substack is a programming error)")
		}
	}()

	parent.PushNested(nil)
}

func TestRecoveryStack_PushNested_UnwindRecurses(t *testing.T) {
	parent := NewRecoveryStack()
	childCompensated := false

	parent.PushNested(tagStack(0, func(int) { childCompensated = true }))

	if err := parent.Unwind(nil); err != nil {
		t.Fatalf("parent.Unwind() error = %v", err)
	}

	if !childCompensated {
		t.Error("child compensate was not invoked by parent.Unwind()")
	}
}

func TestRecoveryStack_MarshalJSON_Empty(t *testing.T) {
	s := NewRecoveryStack()

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	want := `{"entries":[]}`
	if string(data) != want {
		t.Errorf("MarshalJSON = %q, want %q", string(data), want)
	}
}

func TestRecoveryStack_MarshalJSON_Nested(t *testing.T) {
	parent := NewRecoveryStack()
	child := NewRecoveryStack()
	parent.PushNested(child)

	data, err := json.Marshal(parent)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// A nested stack is discriminated structurally by its own `entries` — no `sub` wrapper (phase-8 step 42 slice 3b).
	want := `{"entries":[{"entries":[]}]}`
	if string(data) != want {
		t.Errorf("MarshalJSON = %q, want %q", string(data), want)
	}
}

func TestRecoveryStack_Stamp_RoundTrip(t *testing.T) {
	parent := NewRecoveryStack()

	done := NewChildRecoveryStack(parent)
	done.Stamp("gather#0", "output-0", nil)
	parent.PushNested(done)

	paused := NewChildRecoveryStack(parent)
	paused.Stamp("gather#1", nil, errors.New("paused"))
	parent.PushNested(paused)

	if done.UnitID() != "gather#0" || done.Result() != "output-0" || done.Err() != nil {
		t.Fatalf("stamp accessors = (%q, %v, %v), want (gather#0, output-0, nil)", done.UnitID(), done.Result(), done.Err())
	}

	data, err := json.Marshal(parent)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var reloaded RecoveryStack
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	sub0, ok := reloaded.NestedStackByUnitID("gather#0")
	if !ok {
		t.Fatal("gather#0 substack missing after JSON round-trip")
	}
	if sub0.Result() != "output-0" {
		t.Errorf("sub0.Result() = %v, want output-0", sub0.Result())
	}
	if sub0.Err() != nil {
		t.Errorf("sub0.Err() = %v, want nil", sub0.Err())
	}

	sub1, ok := reloaded.NestedStackByUnitID("gather#1")
	if !ok {
		t.Fatal("gather#1 substack missing after JSON round-trip")
	}
	if sub1.Err() == nil {
		t.Error("sub1.Err() = nil after round-trip, want the paused status preserved")
	}
}

func TestRecoveryStack_NestedStackByUnitID(t *testing.T) {
	parent := NewRecoveryStack()

	parent.PushNested(NewRecoveryStack()) // unstamped — must never match

	stamped := NewChildRecoveryStack(parent)
	stamped.Stamp("iter#2", 42, nil)
	parent.PushNested(stamped)

	if _, ok := parent.NestedStackByUnitID(""); ok {
		t.Error(`NestedStackByUnitID("") matched an unstamped substack; want no match`)
	}
	if _, ok := parent.NestedStackByUnitID("nope"); ok {
		t.Error(`NestedStackByUnitID("nope") matched; want no match`)
	}

	sub, ok := parent.NestedStackByUnitID("iter#2")
	if !ok {
		t.Fatal("iter#2 not found")
	}
	if sub.Result() != 42 {
		t.Errorf("sub.Result() = %v, want 42", sub.Result())
	}
}

// recordingReceipt is a test [Receipt] whose Compensate runs a recorded function — so a test can observe [Unwind]'s
// order and error propagation without a real provider compensating action. Like a real leaf receipt it is its own
// compensator, so [recoveryEntry.compensator] treats it as compensable.
type recordingReceipt struct {
	ReceiptBase
	onCompensate func(*RuntimeEnvironment) error
}

// Compensate runs the recorded function.
func (r *recordingReceipt) Compensate(runtimeEnvironment *RuntimeEnvironment) error {
	return r.onCompensate(runtimeEnvironment)
}

// newRecordingReceipt builds a compensable recording receipt (its own compensator).
func newRecordingReceipt(onCompensate func(*RuntimeEnvironment) error) *recordingReceipt {
	receipt := &recordingReceipt{onCompensate: onCompensate}
	receipt.compensator = receipt
	return receipt
}

// tagStack builds a one-entry nested stack whose receipt's Compensate calls record with tag.
func tagStack(tag int, record func(int)) *RecoveryStack {
	inner := NewRecoveryStack()
	leaf := NewRecoveryStack()
	leaf.entries = append(leaf.entries, recoveryEntry{
		compensator: newRecordingReceipt(func(*RuntimeEnvironment) error { record(tag); return nil }),
	})
	inner.PushNested(leaf)
	return inner
}

// failStack builds a one-entry nested stack whose receipt's Compensate returns err.
func failStack(err error) *RecoveryStack {
	inner := NewRecoveryStack()
	leaf := NewRecoveryStack()
	leaf.entries = append(leaf.entries, recoveryEntry{
		compensator: newRecordingReceipt(func(*RuntimeEnvironment) error { return err }),
	})
	inner.PushNested(leaf)
	return inner
}
