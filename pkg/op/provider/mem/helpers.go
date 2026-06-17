// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// buildCandidate returns an unlinked *Resource for value.
//
// []byte values are dispatched to [newFromBytes]. [io.Reader] values are dispatched to [newFromReader]. String URI
// values are dispatched to [newFromURI]. Resource catalog interaction is the caller's concern, not this function's.
// See [NewResource] and [DiscoverResource].
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - value: []byte, an [io.Reader], or a canonical tag URI string; any other type is an error.
//
// Returns:
//   - *Resource: unlinked candidate.
//   - error: unsupported value type, or an error from the downstream constructor.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	switch v := value.(type) {

	case []byte:
		return newFromBytes(runtimeEnvironment, v)

	case io.Reader:
		return newFromReader(runtimeEnvironment, v)

	case string:
		return newFromURI(runtimeEnvironment, v)

	default:
		return nil, fmt.Errorf("mem.Resource: expected []byte, io.Reader, or URI string, got %T", value)
	}
}

// newFromBytes archives data to the canonical CAS path and returns the resulting *Resource.
//
// Hashes data in memory with SHA-256, derives the CAS URI specific (`sha256:<hex>`), builds the canonical SourcePath
// from that identity, and writes data directly to it. No staging step is needed because the size is known up front.
// The canonical path is computed before the write operation.
//
// Parameters:
//   - runtimeEnvironment: supplies [fsroot.Root] for the canonical path. Must have a non-nil Root.
//   - data: payload bytes; may be empty.
//
// Returns:
//   - *Resource: candidate with Hash populated and content archived at the canonical CAS path.
//   - error: identity construction failure, parent directory creation failure, or write failure.
func newFromBytes(runtimeEnvironment *op.RuntimeEnvironment, data []byte) (*Resource, error) {

	sum := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(sum[:])

	base, err := op.NewResourceBase(runtimeEnvironment, "sha256:"+hexDigest, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	r := &Resource{ResourceBase: base, Hash: hexDigest}

	root := runtimeEnvironment.Root
	canonical := r.SourcePath()

	if err := root.MkdirAll(root.NewPath(filepath.Dir(canonical.Rel())), 0o700); err != nil {
		return nil, fmt.Errorf("mem.Resource: create canonical dir: %w", err)
	}

	if err := root.WriteFile(canonical, data, 0o600); err != nil {
		return nil, fmt.Errorf("mem.Resource: write content: %w", err)
	}

	return r, nil
}

// newFromReader archives a stream to the canonical CAS path and returns the resulting *Resource.
//
// The reader is drained through an [io.TeeReader] into a SHA-256 hasher while writing to a staging file under:
//
//	<Root>/.devlore/mem/resource/.staging/
//
// Once the digest is known, the canonical path is built from the digest and the staging file is renamed onto it:
//
//	<Root>/.devlore/mem/resource/<algo>/<hex[0:2]>/<hex>
//
// The staging file is removed if any step before the rename fails.
//
// The rename overwrites the canonical path atomically on Unix when another producer has already written the same
// content; the bytes are identical by hash equality. Windows behavior differs and is not handled here.
//
// Parameters:
//   - runtimeEnvironment: supplies [fsroot.Root] for the staging and canonical paths. Must have a non-nil Root.
//   - reader: source of payload bytes; drained completely.
//
// Returns:
//   - *Resource: candidate with Hash populated and content archived at the canonical CAS path.
//   - error: staging name generation, staging directory creation, open/copy failure, identity construction failure,
//     canonical directory creation failure, or rename failure.
func newFromReader(runtimeEnvironment *op.RuntimeEnvironment, reader io.Reader) (*Resource, error) {

	root := runtimeEnvironment.Root

	staging, err := stagingPath(root)
	if err != nil {
		return nil, err
	}

	if err := root.MkdirAll(root.NewPath(filepath.Dir(staging.Rel())), 0o700); err != nil {
		return nil, fmt.Errorf("mem.Resource: create staging dir: %w", err)
	}

	promoted := false

	defer func() {
		if !promoted {
			_ = root.Remove(staging)
		}
	}()

	hexDigest, err := streamToStaging(root, staging, reader)
	if err != nil {
		return nil, err
	}

	base, err := op.NewResourceBase(runtimeEnvironment, "sha256:"+hexDigest, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	r := &Resource{ResourceBase: base, Hash: hexDigest}
	canonical := r.SourcePath()

	if err := root.MkdirAll(root.NewPath(filepath.Dir(canonical.Rel())), 0o700); err != nil {
		return nil, fmt.Errorf("mem.Resource: create canonical dir: %w", err)
	}

	if err := root.Rename(staging, canonical); err != nil {
		return nil, fmt.Errorf("mem.Resource: promote staging: %w", err)
	}

	promoted = true
	return r, nil
}

// newFromURI rehydrates a metadata-only *Resource from a canonical tag URI.
//
// The URI's <specific> portion must be `<algo>:<hex>` (currently only `sha256:` is supported). The digest is stamped on
// the returned Resource's Hash field. No content is archived; [Resource.Reader] will succeed only if the canonical CAS
// path already exists on disk from a prior construction.
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - uri: canonical tag URI; <specific> must be `<algo>:<hex>` with `algo == "sha256"` (deferred or malformed URIs
//     are rejected).
//
// Returns:
//   - *Resource: metadata-only Resource with Hash populated.
//   - error: malformed URI, deferred (empty <specific>) URI, missing colon, unsupported algorithm, malformed hex, or
//     [op.ResourceBase] construction failure.
func newFromURI(runtimeEnvironment *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("mem.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	algo, hexPart, ok := strings.Cut(specific, ":")
	if !ok {
		return nil, fmt.Errorf("mem.Resource: URI specific %q is not in <algo>:<hex> form", specific)
	}

	if algo != "sha256" {
		return nil, fmt.Errorf("mem.Resource: unsupported digest algorithm %q (want sha256)", algo)
	}

	if _, err := hex.DecodeString(hexPart); err != nil {
		return nil, fmt.Errorf("mem.Resource: invalid digest hex %q: %w", hexPart, err)
	}

	base, err := op.NewResourceBase(runtimeEnvironment, specific, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	return &Resource{
		ResourceBase: base,
		Hash:         hexPart,
	}, nil
}

// splitTypeID splits a canonical Go type id "<pkgPath>.<TypeName>" into its terminal package segment and type name.
//
// Examples:
//
//	"github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource" → ("mem", "Resource")
//	"mem.Resource" → ("mem", "Resource")
//	"Resource" → ("", "Resource")
//
// Parameters:
//   - typeID: canonical Go type id, typically read from a URI fragment.
//
// Returns:
//   - pkg: terminal segment of the package path; empty when typeID contains no dot.
//   - typeName: unqualified type name; equals typeID when typeID contains no dot.
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

// stagingPath returns a fresh staging path under <Root>/.devlore/mem/resource/.staging/.
//
// The basename is a 16-byte random hex token from [crypto/rand], chosen to be collision-free against the expected
// concurrency of producers.
//
// Parameters:
//   - fsroot: filesystem root under which the staging directory lives.
//
// Returns:
//   - fsroot.Path: staging path with a random hex basename.
//   - error: any failure from [crypto/rand.Read].
func stagingPath(root fsroot.Root) (fsroot.Path, error) {

	var bytes [16]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return fsroot.Path{}, fmt.Errorf("mem.Resource: generate staging name: %w", err)
	}

	return root.NewPath(filepath.Join(".devlore", "mem", "resource", ".staging", hex.EncodeToString(bytes[:]))), nil
}

// streamToStaging drains reader into staging while computing the SHA-256, and returns the lowercase hex digest.
//
// The staging file is opened with O_CREATE|O_EXCL so a name collision is an error rather than a silent overwrite. The
// file is closed via a deferred call. A close error replaces a nil err on return.
//
// Parameters:
//   - fsroot: filesystem root under which the staging file is opened.
//   - staging: staging path produced by [stagingPath].
//   - reader: source of payload bytes; drained completely.
//
// Returns:
//   - string: SHA-256 of the streamed content, lowercase hex.
//   - error: open failure, copy failure, or close failure.
func streamToStaging(root fsroot.Root, staging fsroot.Path, reader io.Reader) (_ string, err error) {

	f, err := root.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("mem.Resource: open staging file: %w", err)
	}

	defer iox.Close(&err, f)

	h := sha256.New()

	if _, err := io.Copy(f, io.TeeReader(reader, h)); err != nil {
		return "", fmt.Errorf("mem.Resource: stage stream: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
