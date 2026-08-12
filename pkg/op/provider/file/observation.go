// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"os"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [*Resource] at the moment it was observed.
//
// Distinct from [Resource], which carries identity only. An observation is a point-in-time metadata snapshot record —
// not a [Resource], never cataloged — whose identity comes from the resource it references
// ([op.ObservationBase.OfResource], by pointer value). It embeds [op.ObservationBase] (the back-link +
// [op.ObservationBase.Exists]) and adds the file-specific measurement fields: `Size`, `Mode`, `ModTime`, `Inode`,
// `Device`.
type Observation struct {
	op.ObservationBase

	// Size is the file size in bytes at observation time. Zero when `Exists` is false.
	Size int64

	// Mode is the file mode bits at observation time. Zero when `Exists` is false.
	Mode os.FileMode

	// ModTime is the file modification time at observation time. Zero value when `Exists` is false.
	ModTime time.Time

	// Inode is the filesystem inode number at observation time. Zero when `Exists` is false or on
	// platforms that do not expose inode information.
	Inode uint64

	// Device is the filesystem device id at observation time. Zero when `Exists` is false or on
	// platforms that do not expose device information.
	Device uint64
}

// NewObservation constructs a *Observation anchored to the resource it observes.
//
// Parameters:
//   - `ofResource`: the [Entry] this observation is of. Must be non-nil (asserted by [op.NewObservationBase]).
//   - `exists`: true when the file existed at observation time.
//   - `size`: file size at observation time.
//   - `mode`: file mode bits at observation time.
//   - `modTime`: file modification time at observation time.
//   - `inode`: filesystem inode at observation time.
//   - `device`: filesystem device id at observation time.
//
// Returns:
//   - `*Observation`: the constructed observation.
func NewObservation(
	ofResource Entry,
	exists bool,
	size int64,
	mode os.FileMode,
	modTime time.Time,
	inode uint64,
	device uint64,
) *Observation {

	return &Observation{
		ObservationBase: op.NewObservationBase(ofResource, exists),
		Size:            size,
		Mode:            mode,
		ModTime:         modTime,
		Inode:           inode,
		Device:          device,
	}
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - `string`: `file.Observation{of=<OfResource.URI()>, exists=<bool>, size=<bytes>, mode=<mode>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("file.Observation{of=%s, exists=%t, size=%d, mode=%v}",
		o.OfResource.URI(), o.Exists, o.Size, o.Mode)
}

// endregion

// endregion
