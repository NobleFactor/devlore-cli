// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// The run index is the store's append-only timeline: one NDJSON line per store write, appended by [WriteGraph]
// and [WriteTrace] beside their document writes. It serves two consumers: tool/scope lookup without opening
// every graph document, and missing-piece detection — an index entry whose document has been deleted out from
// under the store is reportable, where bare directory enumeration would degrade silently. The index is a
// detection hint, never a source of truth: documents absent from the index (pre-index history, a recreated
// index) still count, and a torn final line — a crash mid-append — reads as absent rather than failing the read.

// IndexEventGraph marks an [IndexEntry] recording a graph write; IndexEventTrace marks one recording a trace
// write.
const (
	IndexEventGraph = "graph"
	IndexEventTrace = "trace"
)

// IndexEntry is one line of the run index.
//
// A graph event carries `Tool` and `Scope` (from the graph's origin) so readers can filter without opening the
// document; a trace event carries `TraceFile` and joins to its graph event through the shared `GraphChecksum`.
type IndexEntry struct {

	// At is the UTC moment the store write happened.
	At time.Time `json:"at"`

	// Event is [IndexEventGraph] or [IndexEventTrace].
	Event string `json:"event"`

	// Tool is the producing program's name from the graph's origin; graph events only.
	Tool string `json:"tool,omitempty"`

	// Scope is the planning scope from the graph's origin; graph events only.
	Scope string `json:"scope,omitempty"`

	// GraphChecksum is the graph's canonical "sha256:<hex>" identity — the join key between events.
	GraphChecksum string `json:"graph_checksum"`

	// TraceFile is the trace's filename within its per-graph traces subdirectory; trace events only.
	TraceFile string `json:"trace_file,omitempty"`
}

// IndexPath returns the run index's path at the store root.
//
// Returns:
//   - `string`: the absolute path of `index.ndjson` under the devlore state home.
func IndexPath() string {
	return filepath.Join(devlore.StateHome(), "index.ndjson")
}

// ReadIndex reads the run index, tolerating a torn final line.
//
// Lines that fail to parse are skipped — a crash mid-append must not fail every later read. A missing index
// file is an error (callers distinguish it via [os.IsNotExist]): per the deploy-family design, `writ status`
// treats a missing index as a hard error rather than degrading silently.
//
// Returns:
//   - `[]IndexEntry`: the parsed entries in append order.
//   - `error`: non-nil when the index cannot be opened, including when it does not exist.
func ReadIndex() ([]IndexEntry, error) {

	file, err := os.Open(IndexPath())
	if err != nil {
		return nil, fmt.Errorf("read run index %s: %w", IndexPath(), err)
	}
	defer file.Close() //nolint:errcheck // read-only descriptor

	var entries []IndexEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry IndexEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read run index %s: %w", IndexPath(), err)
	}

	return entries, nil
}

// appendIndexEntry appends one NDJSON line to the run index, creating the file on first write.
//
// Parameters:
//   - `entry`: the event to record.
//
// Returns:
//   - `error`: non-nil when the state home cannot be created or the append fails.
func appendIndexEntry(entry IndexEntry) (err error) {

	if err = os.MkdirAll(devlore.StateHome(), 0o700); err != nil {
		return fmt.Errorf("create state home for run index: %w", err)
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode run index entry: %w", err)
	}

	file, err := os.OpenFile(IndexPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open run index %s: %w", IndexPath(), err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close run index %s: %w", IndexPath(), closeErr)
		}
	}()

	if _, err = file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append run index %s: %w", IndexPath(), err)
	}

	return nil
}
