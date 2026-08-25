// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// ContentAddressedPath returns the sharded on-disk location of a content-addressed resource's stored bytes.
//
// The layout is `.devlore/<package>/<type>/<algo>/<first-two-hex>/<hex>`, derived entirely from the resource's
// own base: the run's root, the `<algo>:<hex>` reachability URI, and the type id. It therefore computes the
// path for WHATEVER resource it is handed — a function pack lands under `.devlore/function/resource/` because
// function's type id says so, not because any particular provider placed it there.
//
// That generality is why this lives here rather than in the provider that happened to need it first. `.devlore`
// is the framework's directory — [RecoverySite] already owns `.devlore/recovery` — and the parameter is
// [Resource], so no provider has to depend on another to find its own content.
//
// Composition goes through [fsroot], which owns path and root questions; nothing here joins strings by hand.
//
// Parameters:
//   - `resource`: any resource whose reachability URI is `<algo>:<hex>`.
//
// Returns:
//   - `fsroot.Path`: the content-addressed path, or the zero Path when there is no root or the URI is not
//     in `<algo>:<hex>` form. A zero Path is the caller's signal that this resource is not content-addressed.
func ContentAddressedPath(resource Resource) fsroot.Path {

	if resource == nil {
		return fsroot.Path{}
	}

	runtimeEnvironment := resource.RuntimeEnvironment()

	if runtimeEnvironment == nil || !runtimeEnvironment.HasRoot() {
		return fsroot.Path{}
	}

	// Through the sealed base accessor: [Resource] does not declare ReachabilityURI, and widening the public
	// interface for one framework caller would be the wrong trade. This is what the sealed accessor is for.
	algo, hexPart, ok := strings.Cut(resource.resourceBase().ReachabilityURI(), ":")
	if !ok {
		return fsroot.Path{}
	}

	shard := hexPart

	if len(shard) >= 2 {
		shard = hexPart[0:2]
	}

	packageName, typeName := splitTypeID(resource.ResourceType())

	return runtimeEnvironment.Root().NewPath(
		".devlore", packageName, strings.ToLower(typeName), algo, shard, hexPart)
}

// ContentAddressedReader opens a content-addressed resource's stored bytes, memory-mapped.
//
// Each call opens a new mmap; the caller must Close the returned reader, which unmaps the file. The content is
// never held in the Go heap.
//
// Parameters:
//   - `resource`: any resource whose stored bytes live at [ContentAddressedPath].
//
// Returns:
//   - `io.ReadCloser`: a reader over the full archived content.
//   - `error`: no content-addressed path (nothing archived), or the mapping failed.
func ContentAddressedReader(resource Resource) (io.ReadCloser, error) {

	abs := ContentAddressedPath(resource).Abs()

	if abs == "" {
		return nil, errors.New("op.ContentAddressedReader: no content-addressed path — the resource was never archived")
	}

	mapped, err := mmap.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("op.ContentAddressedReader: mmap %s: %w", abs, err)
	}

	return &contentAddressedReader{
		mmap:    mapped,
		section: io.NewSectionReader(mapped, 0, int64(mapped.Len())),
	}, nil
}

// region SUPPORTING TYPES

// contentAddressedReader is the mmap-backed [io.ReadCloser] returned by [ContentAddressedReader].
type contentAddressedReader struct {

	// mmap is the underlying memory map; held so Close can unmap it.
	mmap *mmap.ReaderAt

	// section is an [io.SectionReader] over the full range of mmap, used for Read.
	section *io.SectionReader
}

// Read fills p from the mapped content.
//
// Parameters:
//   - `p`: destination buffer.
//
// Returns:
//   - `int`: bytes read.
//   - `error`: io.EOF at the end of the content, or a read failure.
func (r *contentAddressedReader) Read(p []byte) (int, error) { return r.section.Read(p) }

// Close unmaps the underlying file.
//
// Returns:
//   - `error`: non-nil when unmapping fails.
func (r *contentAddressedReader) Close() error { return r.mmap.Close() }

// endregion

// region HELPER FUNCTIONS

// splitTypeID splits a canonical type id into its package name and type name.
//
// `github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource` yields `mem` and `Resource` — the last
// path segment rather than the full import path, so the on-disk layout stays short and readable.
//
// Parameters:
//   - `typeID`: a canonical type id, as [Resource.ResourceType] reports it.
//
// Returns:
//   - `pkg`: the package's last path segment, or "" when the id carries no package.
//   - `typeName`: the type's bare name.
func splitTypeID(typeID string) (pkg, typeName string) {

	dot := strings.LastIndex(typeID, ".")

	if dot < 0 {
		return "", typeID
	}

	typeName = typeID[dot+1:]
	left := typeID[:dot]

	if slash := strings.LastIndex(left, "/"); slash >= 0 {
		pkg = left[slash+1:]
	} else {
		pkg = left
	}

	return pkg, typeName
}

// endregion
