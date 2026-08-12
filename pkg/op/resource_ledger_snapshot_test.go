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
// the step-48 snapshot capture without filesystem fixtures.
type snapshotProbe struct {
	ResourceBase

	etag      string
	digest    Digest
	digestErr error
}

func (p *snapshotProbe) Etag() (string, error) { return p.etag, nil }

func (p *snapshotProbe) Digest() (Digest, error) {
	if p.digestErr != nil {
		return Digest{}, p.digestErr
	}
	return p.digest, nil
}

// newSnapshotProbe interns a probe into `catalog` under `specific` and returns it.
func newSnapshotProbe(t *testing.T, env *RuntimeEnvironment, catalog *ResourceCatalog, specific string) *snapshotProbe {

	t.Helper()

	base, err := NewResourceBase(env, specific, reflect.TypeFor[snapshotProbe]())
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

// TestSnapshot_CapturesContentIdentity pins the step-48 capture: Active entries record both tiers, a digest
// error leaves that field empty (best effort), and Pending / Gone entries record neither.
func TestSnapshot_CapturesContentIdentity(t *testing.T) {

	env := NewRuntimeEnvironment(context.Background(), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))
	catalog := NewResourceCatalog()

	active := newSnapshotProbe(t, env, catalog, "probe:active")
	active.etag = "etag-active"
	active.digest = Digest{Algorithm: "sha256", Bytes: make([]byte, 32)}
	catalog.markActive(active)

	erroring := newSnapshotProbe(t, env, catalog, "probe:erroring")
	erroring.etag = "etag-erroring"
	erroring.digestErr = ErrUnimplemented
	catalog.markActive(erroring)

	pending := newSnapshotProbe(t, env, catalog, "probe:pending")
	pending.etag = "etag-pending"

	gone := newSnapshotProbe(t, env, catalog, "probe:gone")
	gone.etag = "etag-gone"
	catalog.markGone(gone)

	snapshot := catalog.Snapshot()

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

// TestSnapshot_ContentIdentityRoundTrips pins the serialized forms: both tiers survive json and yaml, and
// absent tiers stay absent (omitempty).
func TestSnapshot_ContentIdentityRoundTrips(t *testing.T) {

	env := NewRuntimeEnvironment(context.Background(), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))
	catalog := NewResourceCatalog()

	active := newSnapshotProbe(t, env, catalog, "probe:roundtrip")
	active.etag = "etag-roundtrip"
	active.digest = Digest{Algorithm: "sha256", Bytes: make([]byte, 32)}
	catalog.markActive(active)

	pending := newSnapshotProbe(t, env, catalog, "probe:silent")
	_ = pending

	snapshot := catalog.Snapshot()

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
