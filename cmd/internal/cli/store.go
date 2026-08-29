// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/signing"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// The on-disk execution store keeps graphs and traces as distinct artifacts with a one-graph-to-many-traces
// cardinality. A graph is the immutable plan, persisted once under [GraphsDir] keyed by its checksum; a
// trace is one execution's serialized [op.GraphExecutor] state, persisted per run under [TracesDir] in a
// per-graph subdirectory. A trace ties back to its graph through [op.Trace.GraphChecksum] (== the graph's
// [op.Graph.Checksum]); the shared checksum is also the subdirectory name, so trace→graph lookup is direct.

// storeRoot is the execution store's root, empty when the store lives at its default XDG state path.
//
// A process-wide value, set once from --store during root command setup and read by [GraphsDir] and
// [TracesDir]. A store is a property of the run, not of a call site, and the two directories must move
// together or a trace stops resolving to the definition it ran -- so a single root is the thing that is
// configured, not each directory.
var storeRoot string

// SetStoreRoot points the execution store at root, returning a function that restores the previous value.
//
// An empty root restores the default XDG state path. The returned restore function makes the change safe to
// scope to a test.
//
// Parameters:
//   - `root`: the store's new root directory, or empty for the default.
//
// Returns:
//   - `func()`: restores the root in effect before this call.
func SetStoreRoot(root string) func() {

	previous := storeRoot
	storeRoot = root

	return func() { storeRoot = previous }
}

// StoreHome returns the execution store's root directory.
//
// The store is one anchored tree, not a set of independently located directories. Everything the store owns
// -- the graphs directory, the traces directory, and the run index -- resolves from here, and the same root
// anchors the [fsroot.Dir] the writers open. Relocating a leaf while the anchor stayed behind produced a
// path that escapes its parent, which is the failure this single accessor exists to prevent.
//
// Returns:
//   - `string`: the store root, defaulting to devlore's XDG state home.
func StoreHome() string {

	if storeRoot == "" {
		return devlore.StateHome()
	}

	return storeRoot
}

// storePath resolves a directory inside the execution store.
//
// Parameters:
//   - `name`: the store subdirectory, "graphs" or "traces".
//
// Returns:
//   - `string`: the absolute directory path.
func storePath(name string) string { return filepath.Join(StoreHome(), name) }

// GraphsDir returns the directory holding persisted graphs.
//
// Returns:
//   - `string`: the absolute graphs directory under the devlore state home.
func GraphsDir() string {
	return storePath("graphs")
}

// TracesDir returns the directory holding persisted execution traces.
//
// Traces are grouped into a per-graph subdirectory keyed by graph checksum; see the package store overview.
//
// Returns:
//   - `string`: the absolute traces directory under the devlore state home.
func TracesDir() string {
	return storePath("traces")
}

// WriteGraph persists `graph` under [GraphsDir], keyed by its checksum, and returns the file path.
//
// Idempotent: a graph with the same checksum is written once. Subsequent calls observe the existing file and
// return its path without rewriting — distinct runs of the same plan share one persisted graph. A first write
// also appends an [IndexEventGraph] line to the run index, carrying the origin's tool and scope so index
// readers can filter without opening the document.
//
// Parameters:
//   - `graph`: the assembled, immutable graph to persist. Must not be nil.
//
// Returns:
//   - `string`: the absolute path the graph is stored at.
//   - `error`: non-nil if the directory cannot be created or the graph or its index line cannot be written.
func WriteGraph(graph *op.Graph) (path string, err error) {

	path = filepath.Join(GraphsDir(), safeChecksum(graph.Checksum())+".yaml")

	if _, statErr := os.Stat(path); statErr == nil {
		return path, nil
	}

	// One root for the whole store write. The CLI is the session owner for CLI-side work, so it opens the
	// tree's authority once and every mutation within it — the document, the index line, and for a trace the
	// `latest` symlink — is anchored by the same root (#405, phase 2b).
	stateRoot, err := OpenTree(StoreHome())
	if err != nil {
		return "", err
	}
	defer iox.Close(&err, stateRoot)

	// The store owns its layout, so the store creates it. op.SaveGraph deliberately does not: a save that
	// invents directory structure would be making store decisions on the caller's behalf.
	if err := stateRoot.MkdirAll(stateRoot.NewPath(GraphsDir()), 0o750); err != nil {
		return "", fmt.Errorf("create graphs directory: %w", err)
	}

	signArtifact(graph.Signature() == nil, signing.NamespaceGraph, graph.SignWith)

	// The artifact renders itself: op owns the encoding and the format is stated, not inferred from the
	// ".yaml" on the end of the path (phase-8 step 46 / the single-codec direction).
	if err := op.SaveGraph(stateRoot, stateRoot.NewPath(path), graph, "yaml"); err != nil {
		return "", err
	}

	entry := IndexEntry{At: time.Now().UTC(), Event: IndexEventGraph, GraphChecksum: graph.Checksum()}
	origin := graph.Origin()
	entry.Tool = origin.Tool()
	entry.Scope = origin.Scope()
	if err := appendIndexEntry(stateRoot, entry); err != nil {
		return "", err
	}

	return path, nil
}

// WriteTrace persists `trace` under [TracesDir] in its graph's subdirectory, updates the per-graph
// `latest.yaml` symlink to point at it, and appends an [IndexEventTrace] line to the run index.
//
// Each run writes a distinct timestamped file, so a graph accumulates many traces. The subdirectory is keyed
// by [op.Trace.GraphChecksum]; `latest.yaml` is the convenience entry point for drift detection,
// reconciliation, and pause/restart.
//
// Parameters:
//   - `trace`: the captured executor trace to persist. Must not be nil and must carry a GraphChecksum.
//
// Returns:
//   - `string`: the absolute path the trace is stored at.
//   - `error`: non-nil if the directory cannot be created or the trace/symlink cannot be written.
func WriteTrace(trace *op.Trace) (path string, err error) {

	// One root for the whole store write — see [WriteGraph].
	stateRoot, err := OpenTree(StoreHome())
	if err != nil {
		return "", err
	}
	defer iox.Close(&err, stateRoot)

	directory := filepath.Join(TracesDir(), safeChecksum(trace.GraphChecksum))
	// Nanosecond precision: two runs inside the same second must never overwrite a trace — the store is
	// the audit trail (caught by the deploy scenario, 2026-08-08).
	filename := time.Now().UTC().Format("20060102T150405.000000000Z") + ".yaml"
	path = filepath.Join(directory, filename)

	// The per-graph subdirectory is store layout, so the store creates it — see [WriteGraph].
	if err := stateRoot.MkdirAll(stateRoot.NewPath(directory), 0o750); err != nil {
		return "", fmt.Errorf("create traces directory: %w", err)
	}

	signArtifact(trace.Signature == nil, signing.NamespaceTrace, trace.SignWith)

	// SaveTrace stamps the checksum itself — LoadTrace refuses a document without one, so the pair is total
	// rather than relying on the caller to remember. Signing stays here: the key is the publisher's.
	if err := op.SaveTrace(stateRoot, stateRoot.NewPath(path), trace); err != nil {
		return "", err
	}

	// The index is appended BEFORE the convenience link, because the trace is already durable at this point
	// and the index is what readers enumerate. Linking first meant any link failure discarded the index entry
	// for a trace sitting on disk — silently, since every caller warns rather than fails. That is not a
	// Windows-only defect, but Windows is where it fires for ordinary users: creating a symlink there needs
	// Developer Mode or SeCreateSymbolicLinkPrivilege (#438).
	entry := IndexEntry{
		At:            time.Now().UTC(),
		Event:         IndexEventTrace,
		GraphChecksum: trace.GraphChecksum,
		TraceFile:     filename,
	}
	if err := appendIndexEntry(stateRoot, entry); err != nil {
		return "", err
	}

	// NewPath rebases an absolute path onto the root, so the display string and the root-relative path stay
	// one value rather than two spellings that can drift.
	latest := stateRoot.NewPath(directory, "latest.yaml")
	//nolint:errcheck // diagnose-ignored-error: stale link; see docs/architecture/2.8-eventing-infrastructure.md
	_ = stateRoot.Remove(latest) // best-effort: replace any prior link
	if err := stateRoot.Symlink(filename, latest); err != nil {
		return "", fmt.Errorf("link latest trace %s: %w", latest.Abs(), err)
	}

	return path, nil
}

// LatestTracePath returns the path to the `latest.yaml` symlink for the graph identified by `graphChecksum`.
//
// Parameters:
//   - `graphChecksum`: the graph's checksum (== [op.Trace.GraphChecksum]).
//
// Returns:
//   - `string`: the absolute path to the graph's latest-trace symlink (which may not exist yet).
func LatestTracePath(graphChecksum string) string {
	return filepath.Join(TracesDir(), safeChecksum(graphChecksum), "latest.yaml")
}

// LoadLatestTrace loads the most recent trace for the graph identified by `graphChecksum`.
//
// Parameters:
//   - `graphChecksum`: the graph's checksum (== [op.Trace.GraphChecksum]).
//
// Returns:
//   - *op.Trace: the most recent trace for that graph.
//   - `error`: non-nil if no trace exists for the graph or it cannot be read.
func LoadLatestTrace(graphChecksum string) (*op.Trace, error) {
	return LoadTrace(LatestTracePath(graphChecksum))
}

// LoadTrace loads a single trace from `path`, verifying its tier-1 checksum.
//
// Every trace read funnels through here into [op.LoadTrace] — the checksum trust boundary. A trace with a
// missing or mismatched checksum is refused (docs/architecture/5-graph-trace-integrity.md).
//
// Parameters:
//   - `path`: the trace file to read.
//
// Returns:
//   - *op.Trace: the deserialized, integrity-verified trace.
//   - `error`: non-nil if the file cannot be read, decoded, or verified.
func LoadTrace(path string) (*op.Trace, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace %s: %w", path, err)
	}

	trace, err := op.LoadTrace(data)
	if err != nil {
		return nil, fmt.Errorf("trace %s: %w", path, err)
	}

	return trace, nil
}

// signArtifact signs an unsigned artifact best effort at persist time (phase-8 step 46).
//
// Best effort is the report-tier posture: when no signer resolves (no SSH key and the local key cannot be
// generated), the artifact writes unsigned and verification reports the fact — persistence never fails on
// signing.
//
// Parameters:
//   - `unsigned`: whether the artifact currently carries no signature.
//   - `namespace`: the artifact-kind domain separator.
//   - `signWith`: the artifact's [op.Graph.SignWith]-shaped seam.
func signArtifact(unsigned bool, namespace string, signWith func(func([]byte) (*op.Signature, error)) error) {

	if !unsigned {
		return
	}

	// The CLI is the session owner for CLI-side work, so it opens the root and signing receives one —
	// a leaf never constructs its own filesystem access (#405, phase 2b). OpenTree because the config tree
	// may not exist yet on first use, and opening is a query.
	configRoot, err := OpenTree(devlore.ConfigHome())
	if err != nil {
		return // best effort: the artifact writes unsigned and verification reports the fact
	}

	//nolint:errcheck // diagnose-ignored-error: best-effort signing, and the artifact is already written; see docs/architecture/2.8-eventing-infrastructure.md
	defer configRoot.Close()

	signer, err := signing.DefaultSigner(configRoot, xdg.UserHomePath(".ssh", "id_ed25519"))
	if err != nil {
		return
	}

	_ = signWith(func(canonical []byte) (*op.Signature, error) { //nolint:errcheck // best effort by design
		return signer.Sign(namespace, canonical)
	})
}

// safeChecksum maps a graph checksum ("sha256:<hex>") onto a filesystem-safe path segment by replacing the
// scheme separator, which is invalid in path components on some platforms.
func safeChecksum(checksum string) string {
	return strings.ReplaceAll(checksum, ":", "-")
}
