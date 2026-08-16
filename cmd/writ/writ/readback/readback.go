// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package readback derives "what has writ deployed" from the store's graph and trace documents (phase-8 step 47
// slice 1).
//
// The deployed state is not a file of its own — it is a time-ordered, best-effort fold over the writ-tool runs
// recorded in the store: a run's successful deploying units (link / copy / write / decrypt) mark their targets
// deployed; successful removing units (unlink / remove) mark them gone; failed and undispatched units count for
// nothing. Per-unit file metadata comes from the graph origin's `files` annotation (stamped at plan time by the
// deploy package); per-unit outcomes come from the trace's receipt stack. The store is user territory — deleted
// documents fold as missing pieces (reported, never fatal), but a missing run index is an error per the settled
// design: `writ status` refuses to report from silence.
package readback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Entry is one folded target: the deployed state of one absolute target path.
type Entry struct {

	// Target is the absolute target path — the fold key.
	Target string

	// Source is the absolute source path the target was deployed from.
	Source string

	// Action is the target-producing action name (e.g. "file.link", "file.write_text").
	Action string

	// Scope is the graph origin's scope ("system" / "home", or "" for unscoped runs).
	Scope string

	// Project is the owning project recorded at plan time.
	Project string

	// Layer is the contributing layer recorded at plan time, or "" in single-source mode.
	Layer string

	// TargetRoot is the scope's target root from the run's origin annotations (e.g. $HOME for home) — the
	// confinement root and prune boundary for removal consumers.
	TargetRoot string

	// RecordedEtag is the target's cheap change-detection token captured in the run's ledger snapshot
	// (phase-8 step 48), or "" for runs traced before the capture existed.
	RecordedEtag string

	// RecordedDigest is the target's as-deployed content identity ("<algo>:<hex>") captured in the run's
	// ledger snapshot (phase-8 step 48) — what drift attribution compares against; "" for pre-capture runs.
	RecordedDigest string

	// RecordedSourceDigest is the SOURCE file's content identity from the same snapshot, when the run
	// cataloged the source (encrypted chains use it: the encrypted source's bytes are hashable without
	// decrypting); "" when the source was not cataloged or the run predates the capture.
	RecordedSourceDigest string

	// GraphChecksum identifies the graph whose run last touched this target.
	GraphChecksum string

	// At is the trace timestamp of the run that last touched this target.
	At time.Time
}

// Inventory is the fold's output: the deployed entries plus the store-health findings.
type Inventory struct {

	// Entries maps each deployed absolute target path to its folded state.
	Entries map[string]Entry

	// Packages records the successful package operations (`pkg.*` receipts) writ's runs performed, in fold
	// order — fact-of-record for reporting; package-manager drift is not writ's concern.
	Packages []PackageRecord

	// Findings are the store-health observations: index entries whose documents are gone, and documents the
	// index never recorded. Informational — the fold proceeds over whatever survives.
	Findings []string

	// Runs is the number of traces folded.
	Runs int
}

// PackageRecord is one successful package operation from a writ trace.
type PackageRecord struct {

	// Action is the package action name (e.g. "pkg.install").
	Action string

	// UnitID is the dispatched unit's id within its graph.
	UnitID string

	// GraphChecksum identifies the graph whose run performed the operation.
	GraphChecksum string

	// At is the run's trace timestamp.
	At time.Time
}

// deployingActions marks the target-producing action names; removingActions marks the target-removing ones
// (planned by decommission, slice 2).
var (
	deployingActions = map[string]bool{
		string(encryption.DecryptSopsFile): true,
		string(file.Copy):                  true,
		string(file.Link):                  true,
		string(file.WriteBytes):            true,
		string(file.WriteText):             true,
	}
	removingActions = map[string]bool{
		string(file.Remove): true,
		string(file.Unlink): true,
	}
)

// Fold derives the deployed-state inventory from the store.
//
// Reads the run index (missing index = error, per the settled design), joins trace events to writ-tool graph
// events by checksum, orders the runs by time, and folds each run's per-unit outcomes over its graph's `files`
// annotation. Documents deleted out from under the index fold as findings; traces on disk that the index never
// recorded (pre-index history) fold in via directory enumeration.
//
// Parameters:
//   - `ctx`: the context for the document-loading runtime environment.
//
// Returns:
//   - `*Inventory`: the folded entries, findings, and run count.
//   - `error`: non-nil when the index is missing or the loading environment cannot be built.
func Fold(ctx context.Context) (inventory *Inventory, err error) {

	index, err := cli.ReadIndex()
	if err != nil {
		return nil, err
	}

	var environment *op.RuntimeEnvironment

	environment, err = loadingEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	defer iox.Close(&err, environment)

	inventory = &Inventory{Entries: make(map[string]Entry)}

	runs := collectRuns(index, inventory)

	sort.SliceStable(runs, func(i, j int) bool { return runs[i].at.Before(runs[j].at) })

	for _, run := range runs {
		foldRun(environment, run, inventory)
	}

	return inventory, nil
}

// region SUPPORTING TYPES

// run is one (graph, trace) pair awaiting the fold, ordered by its trace timestamp.
type run struct {

	// checksum is the graph's canonical identity.
	checksum string

	// scope is the graph origin's scope, from the index's graph event.
	scope string

	// tracePath is the trace document's absolute path.
	tracePath string

	// at is the trace's timestamp, parsed from its filename.
	at time.Time
}

// endregion

// region HELPER FUNCTIONS

// collectRuns joins the index's writ-tool graph events with trace events and on-disk trace files.
//
// Index trace events whose files are gone become findings. Trace files on disk that the index never recorded
// fold in (pre-index history); their graph's tool is resolved by loading the graph document lazily during the
// fold. Non-writ tools' runs are excluded.
//
// Parameters:
//   - `index`: the run index entries in append order.
//   - `inventory`: the inventory collecting findings.
//
// Returns:
//   - `[]run`: the runs to fold, unordered.
func collectRuns(index []cli.IndexEntry, inventory *Inventory) []run {

	scopeByChecksum := make(map[string]string)
	toolByChecksum := make(map[string]string)
	for _, entry := range index {
		if entry.Event == cli.IndexEventGraph {
			scopeByChecksum[entry.GraphChecksum] = entry.Scope
			toolByChecksum[entry.GraphChecksum] = entry.Tool
		}
	}

	var runs []run
	seen := make(map[string]bool)

	for _, entry := range index {
		if entry.Event != cli.IndexEventTrace {
			continue
		}
		if tool, known := toolByChecksum[entry.GraphChecksum]; known && tool != "writ" {
			continue
		}

		tracePath := filepath.Join(cli.TracesDir(), safeChecksum(entry.GraphChecksum), entry.TraceFile)
		seen[tracePath] = true

		if _, err := os.Stat(tracePath); err != nil {
			inventory.Findings = append(inventory.Findings,
				fmt.Sprintf("index records trace %s for graph %s, but the document is gone",
					entry.TraceFile, entry.GraphChecksum))
			continue
		}

		runs = append(runs, run{
			checksum:  entry.GraphChecksum,
			scope:     scopeByChecksum[entry.GraphChecksum],
			tracePath: tracePath,
			at:        traceTime(entry.TraceFile, entry.At),
		})
	}

	// Pre-index history: trace files on disk the index never recorded still fold in.
	pattern := filepath.Join(cli.TracesDir(), "*", "*.yaml")
	matches, _ := filepath.Glob(pattern) //nolint:errcheck // the pattern is constant and well-formed
	for _, match := range matches {
		if filepath.Base(match) == "latest.yaml" || seen[match] {
			continue
		}
		checksum := unsafeChecksum(filepath.Base(filepath.Dir(match)))
		if tool, known := toolByChecksum[checksum]; known && tool != "writ" {
			continue
		}
		inventory.Findings = append(inventory.Findings,
			fmt.Sprintf("trace %s is not recorded in the run index", match))
		runs = append(runs, run{
			checksum:  checksum,
			scope:     scopeByChecksum[checksum],
			tracePath: match,
			at:        traceTime(filepath.Base(match), time.Time{}),
		})
	}

	return runs
}

// foldRun applies one run's per-unit outcomes to the inventory.
//
// A missing or unreadable graph document is a finding — the run's outcomes cannot be attributed to targets, so
// the run is skipped. Non-writ graphs discovered at load time (index-unrecorded history) are excluded.
//
// Parameters:
//   - `environment`: the document-loading runtime environment.
//   - `r`: the run to fold.
//   - `inventory`: the inventory to fold into.
func foldRun(environment *op.RuntimeEnvironment, r run, inventory *Inventory) {

	trace, err := cli.LoadTrace(r.tracePath)
	if err != nil {
		inventory.Findings = append(inventory.Findings,
			fmt.Sprintf("trace %s cannot be read: %v", r.tracePath, err))
		return
	}

	graph, err := loadGraph(environment, r.checksum)
	if err != nil {
		inventory.Findings = append(inventory.Findings,
			fmt.Sprintf("graph %s for trace %s is unavailable: %v", r.checksum, r.tracePath, err))
		return
	}

	origin := graph.Origin()
	if origin == nil || origin.Tool() != "writ" {
		return
	}

	metaByUnit := fileMetadata(origin)
	if trace.Stack == nil {
		inventory.Runs++
		return
	}

	scope := r.scope
	if scope == "" {
		scope = origin.Scope()
	}

	targetRoot := ""
	if value, ok := origin.Annotations().Get("target_root"); ok {
		targetRoot = assert.Type[string]("target_root annotation", value)
	}

	recorded := recordedIdentity(trace.Catalog)

	for _, receipt := range trace.Stack.Receipts() {
		if receipt.ForwardAction() == "" || receipt.Err() != nil {
			continue
		}

		if strings.HasPrefix(receipt.ForwardAction(), "pkg.") {
			inventory.Packages = append(inventory.Packages, PackageRecord{
				Action:        receipt.ForwardAction(),
				UnitID:        receipt.UnitID(),
				GraphChecksum: r.checksum,
				At:            r.at,
			})
			continue
		}

		meta, ok := metaByUnit[receipt.UnitID()]
		if !ok {
			continue
		}

		switch {
		case deployingActions[meta.action]:
			identity := recorded[meta.target]
			inventory.Entries[meta.target] = Entry{
				Target:               meta.target,
				Source:               meta.source,
				Action:               meta.action,
				Scope:                scope,
				Project:              meta.project,
				Layer:                meta.layer,
				TargetRoot:           targetRoot,
				RecordedEtag:         identity.etag,
				RecordedDigest:       identity.digest,
				RecordedSourceDigest: recorded[meta.source].digest,
				GraphChecksum:        r.checksum,
				At:                   r.at,
			}
		case removingActions[meta.action]:
			delete(inventory.Entries, meta.target)
		}
	}

	inventory.Runs++
}

// fileMeta is one unit's plan-time file metadata from the graph origin's `files` annotation.
type fileMeta struct {
	target  string
	source  string
	project string
	layer   string
	action  string
}

// fileMetadata extracts the per-unit file metadata from the graph origin's `files` annotation.
//
// The annotation round-trips through the document layer as nested `map[string]any`; extraction is tolerant —
// missing or oddly-typed values yield empty fields rather than errors.
//
// Parameters:
//   - `origin`: the graph origin whose annotations to read.
//
// Returns:
//   - `map[string]fileMeta`: the metadata keyed by unit ID; empty when the annotation is absent.
func fileMetadata(origin op.Origin) map[string]fileMeta {

	value, ok := origin.Annotations().Get("files")
	if !ok {
		return nil
	}

	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	metas := make(map[string]fileMeta, len(raw))
	for unitID, entry := range raw {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		metas[unitID] = fileMeta{
			target:  stringField(fields, "target"),
			source:  stringField(fields, "source"),
			project: stringField(fields, "project"),
			layer:   stringField(fields, "layer"),
			action:  stringField(fields, "action"),
		}
	}
	return metas
}

// contentIdentity is one path's recorded change-detection pair from a run's ledger snapshot.
type contentIdentity struct {
	etag   string
	digest string
}

// recordedIdentity extracts the per-path content identity a run's ledger snapshot recorded (step 48).
//
// File-form entries (URIs whose tag-specific payload is `file://<path>`) map by absolute path; later
// generations of the same path win (append order). A nil catalog — a pre-capture trace — yields an empty map,
// and consumers treat the absent identity as indeterminate.
//
// Parameters:
//   - `catalog`: the trace's ledger snapshot, or nil.
//
// Returns:
//   - `map[string]contentIdentity`: the recorded pairs by absolute path.
func recordedIdentity(catalog *op.ResourceLedgerSnapshot) map[string]contentIdentity {

	recorded := make(map[string]contentIdentity)
	if catalog == nil {
		return recorded
	}

	for _, entry := range catalog.Entries {
		if entry.Etag == "" && entry.Digest == "" {
			continue
		}
		specific, _, err := op.ExtractTagSpecific(entry.URI)
		if err != nil {
			continue
		}
		path, ok := strings.CutPrefix(specific, "file://")
		if !ok {
			continue
		}
		recorded[path] = contentIdentity{etag: entry.Etag, digest: entry.Digest}
	}

	return recorded
}

// ContentDigest renders `data`'s canonical content identity ("sha256:<hex>").
//
// For comparison against a recorded [Entry.RecordedDigest].
//
// Parameters:
//   - `data`: the content to hash.
//
// Returns:
//   - `string`: the canonical digest form.
func ContentDigest(data []byte) string {

	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// stringField reads a string value from a decoded annotation map, tolerating absence.
//
// The map decodes a graph annotation, and graphs are checksum-verified on load ([op.LoadGraph]), so a present value
// of the wrong type can only be a serialization bug — it panics via [assert.Type] rather than degrading silently.
//
// Parameters:
//   - `fields`: the decoded map.
//   - `key`: the field to read.
//
// Returns:
//   - `string`: the value, or "" when absent.
func stringField(fields map[string]any, key string) string {

	value, ok := fields[key]
	if !ok {
		return ""
	}
	return assert.Type[string]("files annotation field "+key, value)
}

// loadGraph loads a graph document from the store by checksum.
//
// Parameters:
//   - `environment`: the document-loading runtime environment.
//   - `checksum`: the graph's canonical "sha256:<hex>" identity.
//
// Returns:
//   - `*op.Graph`: the loaded graph.
//   - `error`: non-nil when the document is missing or fails to load.
func loadGraph(environment *op.RuntimeEnvironment, checksum string) (*op.Graph, error) {

	path := filepath.Join(cli.GraphsDir(), safeChecksum(checksum)+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return op.LoadGraph(environment, data, "yaml")
}

// loadingEnvironment builds the runtime environment graph loading resolves receivers against.
//
// Parameters:
//   - `ctx`: the context the environment carries.
//
// Returns:
//   - `*op.RuntimeEnvironment`: the environment, rooted at "/" (loading reads no files through it).
//   - `error`: non-nil when the root cannot be opened.
func loadingEnvironment(ctx context.Context) (*op.RuntimeEnvironment, error) {

	return op.NewRuntimeEnvironment(ctx, op.NewRuntimeEnvironmentSpec("writ").
		WithRoot(string(filepath.Separator), fsroot.ModeConfined).
		WithApplication(&application.Application{Name: "writ"}))
}

// traceTime parses a trace filename's UTC timestamp, falling back to the index timestamp.
//
// Parameters:
//   - `filename`: the trace filename (e.g. "20260715T183021Z.yaml").
//   - `fallback`: the index entry's timestamp, used when the filename does not parse.
//
// Returns:
//   - `time.Time`: the parsed or fallback timestamp.
func traceTime(filename string, fallback time.Time) time.Time {

	stamp := filename
	if ext := filepath.Ext(stamp); ext != "" {
		stamp = stamp[:len(stamp)-len(ext)]
	}

	if parsed, err := time.Parse("20060102T150405Z", stamp); err == nil {
		return parsed
	}
	return fallback
}

// safeChecksum maps a graph checksum ("sha256:<hex>") onto its filesystem-safe form (the store's convention);
// unsafeChecksum reverses it.
func safeChecksum(checksum string) string {
	return strings.ReplaceAll(checksum, ":", "-")
}

// unsafeChecksum restores a filesystem-safe checksum segment to the canonical "sha256:<hex>" form.
func unsafeChecksum(segment string) string {
	return strings.Replace(segment, "-", ":", 1)
}

// endregion
