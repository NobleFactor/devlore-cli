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

// Observation captures the runtime-observed state of a [Resource] at the moment it was observed.
//
// Distinct from [Resource], which carries identity only (URI and [op.Path] set at construction).
// Observation holds the stat-derived metadata (Size, Mode, ModTime, Inode, Device) plus an Exists
// flag for the "file not found at observation time" case. Each observation is content-addressable:
// the URI is sha256 over the canonical encoding of (`OfURI`, `Exists`, `Size`, `Mode`, `ModTime`,
// `Inode`, `Device`), so two observations with identical contents share an identity.
//
// Observations are first-class [op.Resource] values — they catalog uniformly alongside identity
// Resources and serialize through the same machinery. The framework owns when an observation is
// minted ([Provider.Observe]) and where it lives ([op.ResourceCatalog]); concrete providers never
// touch a [Resource]'s identity surface to update observation state.
type Observation struct {
	op.ResourceBase

	// OfURI is the URI of the [Resource] this observation is of. Captured at observation time so
	// the observation Resource carries a back-reference to its observed identity without needing
	// pointer plumbing.
	OfURI string `json:"of_uri" yaml:"of_uri"`

	// Exists is true when the file existed at observation time. When false, the remaining metadata
	// fields are zero values.
	Exists bool `json:"exists" yaml:"exists"`

	// Size is the file size in bytes at observation time. Zero when `Exists` is false.
	Size int64 `json:"size,omitempty" yaml:"size,omitempty"`

	// Mode is the file mode bits at observation time. Zero when `Exists` is false.
	Mode os.FileMode `json:"mode,omitempty" yaml:"mode,omitempty"`

	// ModTime is the file modification time at observation time. Zero value when `Exists` is false.
	ModTime time.Time `json:"mod_time,omitempty" yaml:"mod_time,omitempty"`

	// Inode is the filesystem inode number at observation time. Zero when `Exists` is false or on
	// platforms that do not expose inode information.
	Inode uint64 `json:"inode,omitempty" yaml:"inode,omitempty"`

	// Device is the filesystem device id at observation time. Zero when `Exists` is false or on
	// platforms that do not expose device information.
	Device uint64 `json:"device,omitempty" yaml:"device,omitempty"`
}

// NewObservation constructs a *Observation with a content-addressable URI derived from its fields.
//
// The URI takes the form `tag:devlore.noblefactor.com,2026-01-01:fileobs:sha256:<hex>#file.Observation`
// where `<hex>` is the lowercase hex of sha256 over the canonical encoding of (`OfURI`, `Exists`,
// `Size`, `Mode`, `ModTime` as Unix nanoseconds, `Inode`, `Device`). Two observations with identical
// contents share a URI.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via [op.NewResourceBase].
//   - `ofURI`: the URI of the [Resource] this observation is of.
//   - `exists`: true when the file existed at observation time.
//   - `size`: file size at observation time.
//   - `mode`: file mode bits at observation time.
//   - `modTime`: file modification time at observation time.
//   - `inode`: filesystem inode at observation time.
//   - `device`: filesystem device id at observation time.
//
// Returns:
//   - *Observation: the constructed observation.
//   - `error`: any [op.NewResourceBase] failure.
func NewObservation(
	runtimeEnvironment *op.RuntimeEnvironment,
	ofURI string,
	exists bool,
	size int64,
	mode os.FileMode,
	modTime time.Time,
	inode uint64,
	device uint64,
) (*Observation, error) {

	specific, err := observationSpecific(ofURI, exists, size, mode, modTime, inode, device)
	if err != nil {
		return nil, err
	}

	base, err := op.NewResourceBase(runtimeEnvironment, specific, reflect.TypeFor[*Observation]())
	if err != nil {
		return nil, fmt.Errorf("file.NewObservation: %w", err)
	}

	return &Observation{
		ResourceBase: base,
		OfURI:        ofURI,
		Exists:       exists,
		Size:         size,
		Mode:         mode,
		ModTime:      modTime,
		Inode:        inode,
		Device:       device,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// Addressing reports [op.AddressingContent] — the URI encodes the observation's content hash, so two
// observations with byte-identical fields share an identity.
//
// Returns:
//   - op.AddressingMode: always [op.AddressingContent].
func (o *Observation) Addressing() op.AddressingMode {
	return op.AddressingContent
}

// Digest returns the observation's content hash as an [op.Digest].
//
// The hash is the same sha256 used to mint the URI's `<specific>` portion — content-addressable
// identity means the URI IS the digest.
//
// Returns:
//   - op.Digest: sha256 algorithm with 32 raw bytes.
//   - `error`: nil; Digest cannot fail for content-addressable observations.
func (o *Observation) Digest() (op.Digest, error) {

	specific, err := observationSpecific(o.OfURI, o.Exists, o.Size, o.Mode, o.ModTime, o.Inode, o.Device)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Observation: digest: %w", err)
	}

	const prefix = "fileobs:sha256:"
	hexPart := specific[len(prefix):]

	raw, err := hex.DecodeString(hexPart)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Observation: digest decode: %w", err)
	}

	return op.Digest{Algorithm: "sha256", Bytes: raw}, nil
}

// Etag returns the observation's URI — for content-addressable Resources the URI itself is the
// change-detection token.
//
// Returns:
//   - string: the canonical URI.
//   - `error`: nil.
func (o *Observation) Etag() (string, error) {
	return o.URI(), nil
}

// Resolve is a no-op for observations.
//
// Observations are terminal — they record what was seen, they do not themselves observe anything
// downstream. Implemented to satisfy [op.Resource].
//
// Returns:
//   - `error`: always nil.
func (o *Observation) Resolve() error {
	return nil
}

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - string: `file.Observation{of=<OfURI>, exists=<bool>, size=<bytes>, mode=<mode>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("file.Observation{of=%s, exists=%t, size=%d, mode=%v}",
		o.OfURI, o.Exists, o.Size, o.Mode)
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// observationSpecific computes the `<specific>` portion of an Observation's URI as
// `fileobs:sha256:<lowercase-hex-of-sha256-over-canonical-encoding>`.
//
// Canonical encoding packs the observation fields little-endian in a fixed order so two observations
// with identical contents hash identically across runs.
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
//   - string: the `fileobs:sha256:<hex>` specific.
//   - `error`: nil; canonical encoding cannot fail today.
func observationSpecific(
	ofURI string,
	exists bool,
	size int64,
	mode os.FileMode,
	modTime time.Time,
	inode uint64,
	device uint64,
) (string, error) {

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

	return "fileobs:sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// endregion
