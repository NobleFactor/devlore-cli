// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"gopkg.in/yaml.v3"
)

// snapshotProbe is a minimal in-package Resource whose content-identity tiers are test-settable, for pinning
// the step-48 snapshot capture without filesystem fixtures. It counts its digest calls (#904) and answers [Tree]
// with `tree`, so one probe stands in for a file or a directory.
type snapshotProbe struct {
	ResourceBase

	etag        string
	digest      Digest
	digestErr   error
	digestCalls int
	tree        bool
}

func (p *snapshotProbe) Etag() (string, error) { return p.etag, nil }

func (p *snapshotProbe) Digest() (Digest, error) {
	p.digestCalls++
	if p.digestErr != nil {
		return Digest{}, p.digestErr
	}
	return p.digest, nil
}

func (p *snapshotProbe) IsTree() bool { return p.tree }

// newSnapshotProbe interns a probe into `catalog` under `specific` and returns it.
func newSnapshotProbe(
	t *testing.T, environment *RuntimeEnvironment, catalog *ResourceCatalog, specific string,
) *snapshotProbe {

	t.Helper()

	base, err := NewResourceBase(environment, specific, reflect.TypeFor[snapshotProbe]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	probe := &snapshotProbe{ResourceBase: base}

	interned, err := catalog.Discover(probe.URI(), func() (Resource, error) { return probe, nil })
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	result, ok := interned.(*snapshotProbe)
	if !ok {
		t.Fatalf("interned resource is %T, want *snapshotProbe", interned)
	}
	return result
}

// newProducedSnapshotProbe interns a probe into `catalog` under `specific` as `producerID`'s production and returns
// it, Active.
func newProducedSnapshotProbe(
	t *testing.T, environment *RuntimeEnvironment, catalog *ResourceCatalog, specific, producerID string,
) *snapshotProbe {

	t.Helper()

	base, err := NewResourceBase(environment, specific, reflect.TypeFor[snapshotProbe]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	probe := &snapshotProbe{ResourceBase: base}

	interned, err := catalog.GetOrCreate(producerID, probe.URI(), func() (Resource, error) { return probe, nil })
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	result, ok := interned.(*snapshotProbe)
	if !ok {
		t.Fatalf("interned resource is %T, want *snapshotProbe", interned)
	}
	return result
}

// newSnapshotEnvironment builds the runtime environment the snapshot tests share.
func newSnapshotEnvironment(t *testing.T) *RuntimeEnvironment {

	t.Helper()

	environment, err := NewRuntimeEnvironment(context.Background(), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}
	return environment
}

// entriesByURI indexes a snapshot's entries by URI.
func entriesByURI(snapshot *ResourceLedgerSnapshot) map[string]LedgerEntrySnapshot {

	byURI := make(map[string]LedgerEntrySnapshot, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		byURI[entry.URI] = entry
	}
	return byURI
}

// TestSnapshot_CapturesContentIdentity pins the step-48 capture.
//
// Active entries record both tiers, a digest error leaves that field empty (best effort), and Pending / Gone entries
// record neither.
func TestSnapshot_CapturesContentIdentity(t *testing.T) {

	environment, err := NewRuntimeEnvironment(context.Background(), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}
	catalog := NewResourceCatalog()

	active := newSnapshotProbe(t, environment, catalog, "probe:active")
	active.etag = "etag-active"
	active.digest = Digest{Algorithm: "sha256", Bytes: make([]byte, 32)}
	catalog.markActive(active)

	erroring := newSnapshotProbe(t, environment, catalog, "probe:erroring")
	erroring.etag = "etag-erroring"
	erroring.digestErr = ErrUnimplemented
	catalog.markActive(erroring)

	pending := newSnapshotProbe(t, environment, catalog, "probe:pending")
	pending.etag = "etag-pending"

	gone := newSnapshotProbe(t, environment, catalog, "probe:gone")
	gone.etag = "etag-gone"
	catalog.markGone(gone)

	snapshot := catalog.Snapshot(nil)

	byURI := make(map[string]LedgerEntrySnapshot, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		byURI[entry.URI] = entry
	}

	activeEntry := byURI[active.URI()]
	if activeEntry.Etag != "etag-active" || activeEntry.Digest != active.digest.String() {
		t.Errorf("active entry = {etag %q, digest %q}, want both tiers recorded", activeEntry.Etag, activeEntry.Digest)
	}

	erroringEntry := byURI[erroring.URI()]
	if erroringEntry.Etag != "etag-erroring" || erroringEntry.Digest != "" {
		t.Errorf("erroring entry = {etag %q, digest %q}, want etag only (digest error is best-effort empty)",
			erroringEntry.Etag, erroringEntry.Digest)
	}

	pendingEntry := byURI[pending.URI()]
	if pendingEntry.Etag != "" || pendingEntry.Digest != "" {
		t.Errorf("pending entry = {etag %q, digest %q}, want neither", pendingEntry.Etag, pendingEntry.Digest)
	}

	goneEntry := byURI[gone.URI()]
	if goneEntry.Etag != "" || goneEntry.Digest != "" {
		t.Errorf("gone entry = {etag %q, digest %q}, want neither", goneEntry.Etag, goneEntry.Digest)
	}
}

// TestSnapshot_ContentIdentityRoundTrips pins the serialized forms.
//
// Both tiers survive json and yaml, and absent tiers stay absent (omitempty).
func TestSnapshot_ContentIdentityRoundTrips(t *testing.T) {

	environment, err := NewRuntimeEnvironment(context.Background(), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}
	catalog := NewResourceCatalog()

	active := newSnapshotProbe(t, environment, catalog, "probe:roundtrip")
	active.etag = "etag-roundtrip"
	active.digest = Digest{Algorithm: "sha256", Bytes: make([]byte, 32)}
	catalog.markActive(active)

	pending := newSnapshotProbe(t, environment, catalog, "probe:silent")
	_ = pending

	snapshot := catalog.Snapshot(nil)

	for name, codec := range map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		"json": {json.Marshal, json.Unmarshal},
		"yaml": {yaml.Marshal, yaml.Unmarshal},
	} {
		t.Run(name, func(t *testing.T) {

			data, err := codec.marshal(snapshot)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var loaded ResourceLedgerSnapshot
			if err := codec.unmarshal(data, &loaded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			byURI := make(map[string]LedgerEntrySnapshot, len(loaded.Entries))
			for _, entry := range loaded.Entries {
				byURI[entry.URI] = entry
			}

			restored := byURI[active.URI()]
			if restored.Etag != "etag-roundtrip" || restored.Digest != active.digest.String() {
				t.Errorf("round-tripped entry = {etag %q, digest %q}, want both tiers intact",
					restored.Etag, restored.Digest)
			}

			silent := byURI[pending.URI()]
			if silent.Etag != "" || silent.Digest != "" {
				t.Errorf("pending entry gained tiers through %s: {etag %q, digest %q}", name, silent.Etag, silent.Digest)
			}
		})
	}
}

// TestSnapshot_UnchangedCatalogCarriesDigestsForward pins #904's comparison.
//
// A second snapshot over an unchanged catalog asks no entry for its digest: every etag matches the prior's, and the
// prior's digest is carried forward intact.
func TestSnapshot_UnchangedCatalogCarriesDigestsForward(t *testing.T) {

	environment := newSnapshotEnvironment(t)
	catalog := NewResourceCatalog()

	probes := make([]*snapshotProbe, 0, 3)
	for _, name := range []string{"probe:one", "probe:two", "probe:three"} {
		probe := newSnapshotProbe(t, environment, catalog, name)
		probe.etag = "etag-" + name
		probe.digest = Digest{Algorithm: "sha256", Bytes: []byte(name)}
		catalog.markActive(probe)
		probes = append(probes, probe)
	}

	first := catalog.Snapshot(nil)
	second := catalog.Snapshot(first)

	firstByURI, secondByURI := entriesByURI(first), entriesByURI(second)
	for _, probe := range probes {
		if probe.digestCalls != 1 {
			t.Errorf("%s: %d digest calls across two snapshots, want 1 (the first)", probe.URI(), probe.digestCalls)
		}
		if got, want := secondByURI[probe.URI()].Digest, firstByURI[probe.URI()].Digest; got != want || got == "" {
			t.Errorf("%s: second snapshot digest %q, want the first's %q carried forward", probe.URI(), got, want)
		}
	}
}

// TestSnapshot_ChangedEtagRecomputesThatEntryAlone pins #904's recompute.
//
// The entry whose etag moved is asked again and records its new digest; the entry whose etag held is not.
func TestSnapshot_ChangedEtagRecomputesThatEntryAlone(t *testing.T) {

	environment := newSnapshotEnvironment(t)
	catalog := NewResourceCatalog()

	moved := newSnapshotProbe(t, environment, catalog, "probe:moved")
	moved.etag = "etag-before"
	moved.digest = Digest{Algorithm: "sha256", Bytes: []byte("before")}
	catalog.markActive(moved)

	held := newSnapshotProbe(t, environment, catalog, "probe:held")
	held.etag = "etag-held"
	held.digest = Digest{Algorithm: "sha256", Bytes: []byte("held")}
	catalog.markActive(held)

	first := catalog.Snapshot(nil)

	moved.etag = "etag-after"
	moved.digest = Digest{Algorithm: "sha256", Bytes: []byte("after")}

	second := entriesByURI(catalog.Snapshot(first))

	if moved.digestCalls != 2 {
		t.Errorf("moved: %d digest calls, want 2 (recomputed after its etag changed)", moved.digestCalls)
	}
	if got := second[moved.URI()].Digest; got != moved.digest.String() {
		t.Errorf("moved: recorded digest %q, want the new %q", got, moved.digest.String())
	}
	if held.digestCalls != 1 {
		t.Errorf("held: %d digest calls, want 1 (its etag held)", held.digestCalls)
	}
	if got := second[held.URI()].Digest; got != held.digest.String() {
		t.Errorf("held: recorded digest %q, want %q carried forward", got, held.digest.String())
	}
}

// TestSnapshot_EntryAbsentFromPriorIsComputed pins #904's new-resource case: an id the prior never saw is asked.
func TestSnapshot_EntryAbsentFromPriorIsComputed(t *testing.T) {

	environment := newSnapshotEnvironment(t)
	catalog := NewResourceCatalog()

	old := newSnapshotProbe(t, environment, catalog, "probe:old")
	old.etag = "etag-old"
	old.digest = Digest{Algorithm: "sha256", Bytes: []byte("old")}
	catalog.markActive(old)

	first := catalog.Snapshot(nil)

	fresh := newSnapshotProbe(t, environment, catalog, "probe:fresh")
	fresh.etag = "etag-fresh"
	fresh.digest = Digest{Algorithm: "sha256", Bytes: []byte("fresh")}
	catalog.markActive(fresh)

	second := entriesByURI(catalog.Snapshot(first))

	if fresh.digestCalls != 1 || second[fresh.URI()].Digest != fresh.digest.String() {
		t.Errorf("fresh: %d digest calls, recorded %q; want 1 call and %q",
			fresh.digestCalls, second[fresh.URI()].Digest, fresh.digest.String())
	}
	if old.digestCalls != 1 {
		t.Errorf("old: %d digest calls, want 1 (carried forward)", old.digestCalls)
	}
}

// TestSnapshot_ErroredPriorDigestIsRecomputed pins #904's error case: an empty prior digest is not carried
// forward as if it were an answer — the entry is asked again.
func TestSnapshot_ErroredPriorDigestIsRecomputed(t *testing.T) {

	environment := newSnapshotEnvironment(t)
	catalog := NewResourceCatalog()

	probe := newSnapshotProbe(t, environment, catalog, "probe:erroring")
	probe.etag = "etag-steady"
	probe.digestErr = ErrUnimplemented
	catalog.markActive(probe)

	first := catalog.Snapshot(nil)
	if got := entriesByURI(first)[probe.URI()].Digest; got != "" {
		t.Fatalf("first snapshot recorded %q for an erroring digest, want empty", got)
	}

	probe.digestErr = nil
	probe.digest = Digest{Algorithm: "sha256", Bytes: []byte("recovered")}

	second := entriesByURI(catalog.Snapshot(first))

	if probe.digestCalls != 2 {
		t.Errorf("%d digest calls, want 2 (the error is not carried forward)", probe.digestCalls)
	}
	if got := second[probe.URI()].Digest; got != probe.digest.String() {
		t.Errorf("recorded digest %q, want %q", got, probe.digest.String())
	}
}

// TestSnapshot_DiscoveredTreeRecordsEtagOnly pins #904's tree rule.
//
// A tree with no producer — a boundary the run found — records its etag and is never asked for a digest; a tree
// the run produced records both tiers.
func TestSnapshot_DiscoveredTreeRecordsEtagOnly(t *testing.T) {

	environment := newSnapshotEnvironment(t)
	catalog := NewResourceCatalog()

	found := newSnapshotProbe(t, environment, catalog, "probe:found-tree")
	found.tree = true
	found.etag = "etag-found"
	found.digest = Digest{Algorithm: "sha256", Bytes: []byte("found")}
	catalog.markActive(found)

	made := newProducedSnapshotProbe(t, environment, catalog, "probe:made-tree", "mkdir-step")
	made.tree = true
	made.etag = "etag-made"
	made.digest = Digest{Algorithm: "sha256", Bytes: []byte("made")}

	byURI := entriesByURI(catalog.Snapshot(nil))

	foundEntry := byURI[found.URI()]
	if foundEntry.ProducerID != "" {
		t.Fatalf("found tree carries producer %q, want none", foundEntry.ProducerID)
	}
	if found.digestCalls != 0 || foundEntry.Etag != "etag-found" || foundEntry.Digest != "" {
		t.Errorf("found tree: %d digest calls, {etag %q, digest %q}; want 0 calls, etag only",
			found.digestCalls, foundEntry.Etag, foundEntry.Digest)
	}

	madeEntry := byURI[made.URI()]
	if madeEntry.ProducerID != "mkdir-step" {
		t.Fatalf("made tree carries producer %q, want %q", madeEntry.ProducerID, "mkdir-step")
	}
	if made.digestCalls != 1 || madeEntry.Etag != "etag-made" || madeEntry.Digest != made.digest.String() {
		t.Errorf("made tree: %d digest calls, {etag %q, digest %q}; want 1 call and both tiers",
			made.digestCalls, madeEntry.Etag, madeEntry.Digest)
	}
}
