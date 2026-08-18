// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeLeaf is a [leaf] whose Update outcome is configurable and recorded, for router fan-out tests.
type fakeLeaf struct {
	typ          string // purl type the fake reports
	updateErr    error  // error Update returns; nil for success
	updateCalled bool   // set when Update runs
	present      bool   // value Present reports
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
func (f *fakeLeaf) Present() bool                                     { return f.present }
func (f *fakeLeaf) Search(string, int) []SearchResult                 { return nil }
func (f *fakeLeaf) Update() error                                     { f.updateCalled = true; return f.updateErr }

// captureRefresh lives in update_unix_test.go: it fakes the unix-scoped runShellCommand for the tagged refresh tests.

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

// fakeRawDriver is a [rawDriver] that is also a [refresher] and [stalenessAware].
//
// A controllable index age and a refresh counter exercise the automatic staleness gate through the real driver verb
// path.
type fakeRawDriver struct {
	typ       string        // purl type the fake reports
	binary    string        // executable name Present looks for on the PATH
	age       time.Duration // index age indexAge reports
	refreshes int           // count of refresh invocations
}

var _ rawDriver = (*fakeRawDriver)(nil)

func (f *fakeRawDriver) name() string                               { return f.typ }
func (f *fakeRawDriver) purlType() string                           { return f.typ }
func (f *fakeRawDriver) executable() string                         { return f.binary }
func (f *fakeRawDriver) installed(string) bool                      { return false }
func (f *fakeRawDriver) version(string) string                      { return "" }
func (f *fakeRawDriver) available(string) bool                      { return true }
func (f *fakeRawDriver) searchRaw(string, int) []SearchResult       { return nil }
func (f *fakeRawDriver) installRaw([]string, map[string]any) Result { return Result{OK: true} }
func (f *fakeRawDriver) removeRaw([]string) Result                  { return Result{OK: true} }
func (f *fakeRawDriver) refresh() Result                            { f.refreshes++; return Result{OK: true} }
func (f *fakeRawDriver) indexAge() time.Duration                    { return f.age }

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

// indexOp is a driver operation exercised against the index gate, named for its failure message.
type indexOp struct {
	name string
	call func(driver)
}

// mutatorOps are the operations that resolve a specific version and so depend on a current index.
var mutatorOps = []indexOp{
	{"Install", func(d driver) { d.Install([]PURL{{Type: "deb", Name: "x"}}, nil) }},
	{"Upgrade", func(d driver) { d.Upgrade([]PURL{{Type: "deb", Name: "x"}}, nil) }},
}

// queryOps are the operations that read the index without naming a version.
var queryOps = []indexOp{
	{"Available", func(d driver) { d.Available(PURL{Type: "deb", Name: "x"}) }},
	{"Search", func(d driver) { d.Search("x", 0) }},
}

// TestMutatorsRefreshAStaleIndex verifies Install and Upgrade still gate on age.
func TestMutatorsRefreshAStaleIndex(t *testing.T) {

	for _, op := range mutatorOps {
		fake := &fakeRawDriver{typ: "deb", age: refreshTTL + time.Hour}
		op.call(newDriver(fake))

		if fake.refreshes != 1 {
			t.Errorf("%s on stale index: refreshes = %d, want 1", op.name, fake.refreshes)
		}
	}
}

// TestQueriesIgnoreAStaleIndex verifies age alone never sends a read to the network.
//
// A stale index still answers an existence or search query well, so paying a refresh for one is the defect this
// gate was reshaped to remove.
func TestQueriesIgnoreAStaleIndex(t *testing.T) {

	for _, op := range queryOps {
		fake := &fakeRawDriver{typ: "deb", age: refreshTTL + time.Hour}
		op.call(newDriver(fake))

		if fake.refreshes != 0 {
			t.Errorf("%s on stale index: refreshes = %d, want 0", op.name, fake.refreshes)
		}
	}
}

// TestQueriesRefreshAMissingIndex verifies absence still refreshes, for either kind of operation.
//
// With no index Available reports false for every package, which a caller cannot distinguish from an
// authoritative "no such package" — so absence is worth the fetch where age is not.
func TestQueriesRefreshAMissingIndex(t *testing.T) {

	for _, ops := range [][]indexOp{queryOps, mutatorOps} {
		for _, op := range ops {
			fake := &fakeRawDriver{typ: "deb", age: unknownIndexAge}
			op.call(newDriver(fake))

			if fake.refreshes != 1 {
				t.Errorf("%s on missing index: refreshes = %d, want 1", op.name, fake.refreshes)
			}
		}
	}
}
