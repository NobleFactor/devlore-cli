// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// The on-disk execution store keeps graphs and traces as distinct artifacts with a one-graph-to-many-traces
// cardinality. A graph is the immutable plan, persisted once under [GraphsDir] keyed by its checksum; a
// trace is one execution's serialized [op.GraphExecutor] state, persisted per run under [ReceiptsDir] in a
// per-graph subdirectory. A trace ties back to its graph through [op.Trace.GraphChecksum] (== the graph's
// [op.Graph.Checksum]); the shared checksum is also the subdirectory name, so trace→graph lookup is direct.

// GraphsDir returns the directory holding persisted graphs.
//
// Returns:
//   - `string`: the absolute graphs directory under the devlore state home.
func GraphsDir() string {
	return filepath.Join(DevloreStateHome(), "graphs")
}

// ReceiptsDir returns the directory holding persisted execution traces.
//
// Traces are grouped into a per-graph subdirectory keyed by graph checksum; see the package store overview.
//
// Returns:
//   - `string`: the absolute receipts directory under the devlore state home.
func ReceiptsDir() string {
	return filepath.Join(DevloreStateHome(), "receipts")
}

// WriteGraph persists `graph` under [GraphsDir], keyed by its checksum, and returns the file path.
//
// Idempotent: a graph with the same checksum is written once. Subsequent calls observe the existing file and
// return its path without rewriting — distinct runs of the same plan share one persisted graph.
//
// Parameters:
//   - `graph`: the assembled, immutable graph to persist. Must not be nil.
//
// Returns:
//   - `string`: the absolute path the graph is stored at.
//   - `error`: non-nil if the directory cannot be created or the graph cannot be written.
func WriteGraph(graph *op.Graph) (string, error) {

	path := filepath.Join(GraphsDir(), safeChecksum(graph.Checksum())+".yaml")

	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	if err := document.Write(path, graph); err != nil {
		return "", fmt.Errorf("write graph %s: %w", path, err)
	}

	return path, nil
}

// WriteTrace persists `trace` under [ReceiptsDir] in its graph's subdirectory and updates the per-graph
// `latest.yaml` symlink to point at it.
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
func WriteTrace(trace *op.Trace) (string, error) {

	directory := filepath.Join(ReceiptsDir(), safeChecksum(trace.GraphChecksum))
	filename := time.Now().UTC().Format("20060102T150405Z") + ".yaml"
	path := filepath.Join(directory, filename)

	if err := document.Write(path, trace); err != nil {
		return "", fmt.Errorf("write trace %s: %w", path, err)
	}

	latest := filepath.Join(directory, "latest.yaml")
	_ = os.Remove(latest) // best-effort: replace any prior link
	if err := os.Symlink(filename, latest); err != nil {
		return "", fmt.Errorf("link latest trace %s: %w", latest, err)
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
	return filepath.Join(ReceiptsDir(), safeChecksum(graphChecksum), "latest.yaml")
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

// LoadTrace loads a single trace from `path`.
//
// Parameters:
//   - `path`: the trace file to read.
//
// Returns:
//   - *op.Trace: the deserialized trace.
//   - `error`: non-nil if the file cannot be read or decoded.
func LoadTrace(path string) (*op.Trace, error) {
	return document.ReadFile[op.Trace](path)
}

// safeChecksum maps a graph checksum ("sha256:<hex>") onto a filesystem-safe path segment by replacing the
// scheme separator, which is invalid in path components on some platforms.
func safeChecksum(checksum string) string {
	return strings.ReplaceAll(checksum, ":", "-")
}
