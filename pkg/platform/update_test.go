// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeLeaf is a [leaf] whose Update outcome is configurable and recorded, for router fan-out tests.
type fakeLeaf struct {
	typ          string
	updateErr    error
	updateCalled bool
}

var _ leaf = (*fakeLeaf)(nil)

func (f *fakeLeaf) name() string                                      { return f.typ }
func (f *fakeLeaf) purlType() string                                  { return f.typ }
func (f *fakeLeaf) Install([]PURL, map[string]any) ([]Receipt, error) { return nil, nil }
func (f *fakeLeaf) Remove([]PURL, map[string]any) ([]Receipt, error)  { return nil, nil }
func (f *fakeLeaf) Upgrade([]PURL, map[string]any) ([]Receipt, error) { return nil, nil }
func (f *fakeLeaf) Installed(PURL) bool                               { return false }
func (f *fakeLeaf) Version(PURL) string                               { return "" }
func (f *fakeLeaf) Available(PURL) bool                               { return false }
func (f *fakeLeaf) Search(string, int) []SearchResult                 { return nil }
func (f *fakeLeaf) Update() error                                     { f.updateCalled = true; return f.updateErr }

// captureRefresh swaps in a recording [runShellCommand], invokes `refresh`, and returns the command string and sudo
// flag it issued. It restores the real command runner on return, so it asserts a leaf's refresh wiring (the command
// and its elevation flag) without shelling out or needing fsroot.
//
// Parameters:
//   - `t`: the test.
//   - `refresh`: the leaf refresh method value to invoke.
//
// Returns:
//   - `string`: the command the refresh issued.
//   - `bool`: the sudo (elevation) flag it requested.
func captureRefresh(t *testing.T, refresh func() PlatformResult) (string, bool) {

	t.Helper()

	var (
		gotCmd  string
		gotSudo bool
	)

	original := runShellCommand
	runShellCommand = func(command string, sudo bool) PlatformResult {
		gotCmd, gotSudo = command, sudo
		return PlatformResult{OK: true}
	}
	defer func() { runShellCommand = original }()

	refresh()

	return gotCmd, gotSudo
}

// TestCompositeUpdateFansOutToEveryLeaf verifies the router invokes Update on every registered leaf.
func TestCompositeUpdateFansOutToEveryLeaf(t *testing.T) {

	leaves := []*fakeLeaf{{typ: "deb"}, {typ: "brew"}, {typ: "rpm"}}
	router := newComposite([]leaf{leaves[0], leaves[1], leaves[2]}, leaves[0])

	if err := router.Update(); err != nil {
		t.Fatalf("Update: unexpected error %v", err)
	}

	for _, l := range leaves {
		if !l.updateCalled {
			t.Errorf("leaf %q: Update was not called", l.typ)
		}
	}
}

// TestCompositeUpdateAggregatesFailures verifies a failing leaf's error surfaces while peers still run.
func TestCompositeUpdateAggregatesFailures(t *testing.T) {

	good := &fakeLeaf{typ: "deb"}
	bad := &fakeLeaf{typ: "brew", updateErr: errors.New("refresh boom")}
	router := newComposite([]leaf{good, bad}, good)

	err := router.Update()

	if err == nil || !strings.Contains(err.Error(), "refresh boom") {
		t.Errorf("Update error = %v, want it to carry the failing leaf's error", err)
	}
	if !good.updateCalled {
		t.Error("a failing peer must not stop the fan-out, but the good leaf was skipped")
	}
}

// fakeRawDriver is a [rawDriver] that is also a [refresher] and [stalenessAware], with a controllable index age and
// a refresh counter, for exercising the automatic staleness gate through the real driver verb path.
type fakeRawDriver struct {
	typ       string
	age       time.Duration
	refreshes int
}

var _ rawDriver = (*fakeRawDriver)(nil)

func (f *fakeRawDriver) name() string                         { return f.typ }
func (f *fakeRawDriver) purlType() string                     { return f.typ }
func (f *fakeRawDriver) installed(string) bool                { return false }
func (f *fakeRawDriver) version(string) string                { return "" }
func (f *fakeRawDriver) available(string) bool                { return true }
func (f *fakeRawDriver) searchRaw(string, int) []SearchResult { return nil }
func (f *fakeRawDriver) installRaw([]string, map[string]any) PlatformResult {
	return PlatformResult{OK: true}
}
func (f *fakeRawDriver) removeRaw([]string) PlatformResult { return PlatformResult{OK: true} }
func (f *fakeRawDriver) refresh() PlatformResult           { f.refreshes++; return PlatformResult{OK: true} }
func (f *fakeRawDriver) indexAge() time.Duration           { return f.age }

// TestEnsureFreshRefreshesStaleIndexBeforeInstall verifies a stale index is refreshed before an index-consuming op.
func TestEnsureFreshRefreshesStaleIndexBeforeInstall(t *testing.T) {

	fake := &fakeRawDriver{typ: "deb", age: refreshTTL + time.Hour}
	newDriver(fake).Install([]PURL{{Type: "deb", Name: "x"}}, nil)

	if fake.refreshes != 1 {
		t.Errorf("stale index before Install: refreshes = %d, want 1", fake.refreshes)
	}
}

// TestEnsureFreshSkipsFreshIndex verifies a fresh index is not refreshed.
func TestEnsureFreshSkipsFreshIndex(t *testing.T) {

	fake := &fakeRawDriver{typ: "deb", age: time.Minute}
	newDriver(fake).Install([]PURL{{Type: "deb", Name: "x"}}, nil)

	if fake.refreshes != 0 {
		t.Errorf("fresh index before Install: refreshes = %d, want 0", fake.refreshes)
	}
}

// TestEnsureFreshIgnoresLocalOps verifies local-state operations never gate a refresh, even when stale.
func TestEnsureFreshIgnoresLocalOps(t *testing.T) {

	fake := &fakeRawDriver{typ: "deb", age: refreshTTL + time.Hour}
	d := newDriver(fake)

	d.Remove([]PURL{{Type: "deb", Name: "x"}}, nil)
	d.Installed(PURL{Type: "deb", Name: "x"})
	d.Version(PURL{Type: "deb", Name: "x"})

	if fake.refreshes != 0 {
		t.Errorf("local ops (Remove/Installed/Version): refreshes = %d, want 0", fake.refreshes)
	}
}

// TestEnsureFreshGatesIndexConsumingOps verifies Upgrade, Search, and Available each gate a stale-index refresh.
func TestEnsureFreshGatesIndexConsumingOps(t *testing.T) {

	cases := []struct {
		name string
		call func(driver)
	}{
		{"Upgrade", func(d driver) { d.Upgrade([]PURL{{Type: "deb", Name: "x"}}, nil) }},
		{"Search", func(d driver) { d.Search("x", 0) }},
		{"Available", func(d driver) { d.Available(PURL{Type: "deb", Name: "x"}) }},
	}

	for _, c := range cases {
		fake := &fakeRawDriver{typ: "deb", age: refreshTTL + time.Hour}
		c.call(newDriver(fake))
		if fake.refreshes != 1 {
			t.Errorf("%s on stale index: refreshes = %d, want 1", c.name, fake.refreshes)
		}
	}
}
