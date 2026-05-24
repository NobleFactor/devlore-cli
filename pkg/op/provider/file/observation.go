// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [*Resource] at the moment it was observed.
//
// Distinct from [Resource], which carries identity only. Observation embeds [op.ObservationBase]
// (which itself embeds [op.ResourceBase] and adds the typed back-link [op.ObservationBase.OfResource]
// + [op.ObservationBase.Exists]) and adds the file-specific measurement fields: `Size`, `Mode`,
// `ModTime`, `Inode`, `Device`. Each observation is content-addressable — the URI is sha256 over
// the canonical encoding of `(OfResource.URI(), Exists, Size, Mode, ModTime, Inode, Device)`.
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

// NewObservation constructs a *Observation with a content-addressable URI derived from its fields.
//
// The URI takes the form `tag:devlore.noblefactor.com,2026-01-01:sha256:<hex>#file.Observation`
// where `<hex>` is lowercase hex of sha256 over the canonical encoding of `(OfResource.URI(),
// Exists, Size, Mode, ModTime, Inode, Device)`. Two observations with identical contents share a
// URI; the catalog deduplicates them naturally.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via [op.NewObservationBase].
//   - `ofResource`: the [*Resource] this observation is of. Must be non-nil (asserted by
//     [op.NewObservationBase]).
//   - `exists`: true when the file existed at observation time.
//   - `size`: file size at observation time.
//   - `mode`: file mode bits at observation time.
//   - `modTime`: file modification time at observation time.
//   - `inode`: filesystem inode at observation time.
//   - `device`: filesystem device id at observation time.
//
// Returns:
//   - *Observation: the constructed observation.
//   - `error`: any [op.NewObservationBase] failure.
func NewObservation(
	runtimeEnvironment *op.RuntimeEnvironment,
	ofResource *Resource,
	exists bool,
	size int64,
	mode os.FileMode,
	modTime time.Time,
	inode uint64,
	device uint64,
) (*Observation, error) {

	specific := observationSpecific(ofResource.URI(), exists, size, mode, modTime, inode, device)

	base, err := op.NewObservationBase(
		runtimeEnvironment,
		specific,
		reflect.TypeFor[*Observation](),
		ofResource,
		exists,
	)
	if err != nil {
		return nil, fmt.Errorf("file.NewObservation: %w", err)
	}

	return &Observation{
		ObservationBase: base,
		Size:            size,
		Mode:            mode,
		ModTime:         modTime,
		Inode:           inode,
		Device:          device,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - string: `file.Observation{of=<OfResource.URI()>, exists=<bool>, size=<bytes>, mode=<mode>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("file.Observation{of=%s, exists=%t, size=%d, mode=%v}",
		o.OfResource.URI(), o.Exists, o.Size, o.Mode)
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// observationSpecific computes the `<specific>` portion of an Observation's URI as
// `sha256:<lowercase-hex-of-canonical-encoding>`.
//
// Canonical encoding packs the observation fields little-endian in a fixed order so two
// observations with identical contents hash identically across runs. The typeID fragment
// (`#file.Observation`) carries the type discriminator.
//
// Parameters:
//   - `ofURI`: the URI of the [Resource] this observation is of.
//   - `exists`: true when the file existed at observation time.
//   - `size`: file size at observation time.
//   - `mode`: file mode bits at observation time.
//   - `modTime`: file modification time at observation time.
//   - `inode`: filesystem inode at observation time.
//   - `device`: filesystem device id at observation time.
//
// Returns:
//   - string: the `sha256:<hex>` specific.
func observationSpecific(
	ofURI string,
	exists bool,
	size int64,
	mode os.FileMode,
	modTime time.Time,
	inode uint64,
	device uint64,
) string {

	var buf [41]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(size)) //nolint:gosec // file sizes are non-negative.
	binary.LittleEndian.PutUint32(buf[8:12], uint32(mode))
	binary.LittleEndian.PutUint64(buf[12:20], uint64(modTime.UnixNano()))
	binary.LittleEndian.PutUint64(buf[20:28], inode)
	binary.LittleEndian.PutUint64(buf[28:36], device)
	if exists {
		buf[36] = 1
	}

	h := sha256.New()
	h.Write([]byte(ofURI))
	h.Write(buf[:37])

	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// endregion
