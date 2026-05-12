// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// buildCandidate returns an unlinked *Resource for value.
//
// [ResourceSpec] values are dispatched to [newFromSpec]. String URI values are dispatched to [newFromURI]. [io.Reader]
// values are dispatched to [newFromReader] for content-addressed archival. Resource catalog interaction is the caller's
// concern, not this function's. See [NewResource] and [DiscoverResource].
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - value: a [ResourceSpec], a canonical tag URI string, or an [io.Reader]; any other type is an error.
//
// Returns:
//   - *Resource: unlinked candidate.
//   - error: unsupported value type, or an error from the downstream constructor.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	switch v := value.(type) {

	case ResourceSpec:
		return newFromSpec(runtimeEnvironment, v)

	case string:
		return newFromURI(runtimeEnvironment, v)

	case io.Reader:
		return newFromReader(runtimeEnvironment, v)

	default:
		return nil, fmt.Errorf("mem.Resource: expected ResourceSpec, URI string, or io.Reader, got %T", value)
	}
}

// newFromReader archives a stream to the canonical CAS path and returns the resulting *Resource.
//
// The reader is drained through an [io.TeeReader] into a SHA-256 hasher while writing to a staging file under
// <Root>/.devlore/mem/resource/.staging/. Once the digest is known, the canonical path
// <Root>/.devlore/mem/resource/<algo>/<hex[0:2]>/<hex> is built from the digest and the staging file is renamed onto
// it. The staging file is removed if any step before the rename fails.
//
// The rename overwrites the canonical path atomically on Unix when another producer has already written the same
// content; the bytes are identical by hash equality. Windows behavior differs and is not handled here.
//
// Parameters:
//   - runtimeEnvironment: supplies [op.Root] for the staging and canonical paths.
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

// newFromSpec builds an unlinked *Resource from spec and archives spec.Data to disk when non-nil.
//
// At least one of spec.Namespace and spec.Name must be non-empty; an empty spec is rejected.
//
// Parameters:
//   - runtimeEnvironment: supplies [op.Root] for archival and threads into the produced [op.ResourceBase].
//   - spec: identity (Namespace, Name) plus optional payload (Data).
//
// Returns:
//   - *Resource: candidate with ResourceBase, Namespace, and Name populated; Hash populated when Data was archived.
//   - error: empty spec, [op.ResourceBase] construction failure, or [writeSpecData] failure.
func newFromSpec(runtimeEnvironment *op.RuntimeEnvironment, spec ResourceSpec) (*Resource, error) {

	if spec.Namespace == "" && spec.Name == "" {
		return nil, fmt.Errorf("mem.Resource: spec must have non-empty Namespace or Name")
	}

	base, err := op.NewResourceBase(runtimeEnvironment, spec.Specific(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	r := &Resource{
		ResourceBase: base,
		Namespace:    spec.Namespace,
		Name:         spec.Name,
	}

	if spec.Data != nil {
		hash, err := writeSpecData(runtimeEnvironment.Root, r.SourcePath(), spec.Data)
		if err != nil {
			return nil, fmt.Errorf("mem.Resource: %w", err)
		}
		r.Hash = hash
	}

	return r, nil
}

// newFromURI rehydrates a metadata-only *Resource from a canonical tag URI.
//
// The URI's <specific> portion is parsed as "<ns>/<name>" (slash-split) or as a bare "<name>" (no slash) and populates
// the corresponding fields. No content is archived; Hash is empty and [Resource.Reader] will fail until the content is
// archived by another path.
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - uri: canonical tag URI; the <specific> portion must be non-empty (deferred URIs are rejected).
//
// Returns:
//   - *Resource: metadata-only Resource.
//   - error: malformed URI, deferred (empty <specific>) URI, or [op.ResourceBase] construction failure.
func newFromURI(runtimeEnvironment *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("mem.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	base, err := op.NewResourceBase(runtimeEnvironment, specific, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	r := &Resource{
		ResourceBase: base,
	}

	if ns, name, ok := strings.Cut(specific, "/"); ok {
		r.Namespace = ns
		r.Name = name
	} else {
		r.Name = specific
	}

	return r, nil
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
//   - root: filesystem root under which the staging directory lives.
//
// Returns:
//   - op.Path: staging path with a random hex basename.
//   - error: any failure from [crypto/rand.Read].
func stagingPath(root op.Root) (op.Path, error) {

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return op.Path{}, fmt.Errorf("mem.Resource: generate staging name: %w", err)
	}

	return root.NewPath(filepath.Join(".devlore", "mem", "resource", ".staging", hex.EncodeToString(b[:]))), nil
}

// streamToStaging drains reader into staging while computing the SHA-256 in-flight via [io.TeeReader] and returns the
// lowercase hex digest.
//
// The staging file is opened with O_CREATE|O_EXCL so a name collision is an error rather than silent overwrite. The
// file is closed via a deferred call; a close error replaces a nil err on return.
//
// Parameters:
//   - root: filesystem root under which the staging file is opened.
//   - staging: staging path produced by [stagingPath].
//   - reader: source of payload bytes; drained completely.
//
// Returns:
//   - string: SHA-256 of the streamed content, lowercase hex.
//   - error: open failure, copy failure, or close failure.
func streamToStaging(root op.Root, staging op.Path, reader io.Reader) (_ string, err error) {

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

// writeBytes writes data to sourcePath under root and returns the SHA-256 hex of the bytes.
//
// The parent directory is not created — callers (notably [writeSpecData]) ensure it exists.
//
// Parameters:
//   - root: filesystem root under which the file is written.
//   - sourcePath: destination path.
//   - data: payload bytes.
//
// Returns:
//   - string: SHA-256 of data, lowercase hex.
//   - error: any failure from [op.Root.WriteFile].
func writeBytes(root op.Root, sourcePath op.Path, data []byte) (string, error) {

	if err := root.WriteFile(sourcePath, data, 0o600); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// writeSpecData archives data to sourcePath, dispatching on data's runtime type.
//
// The parent directory is created before writing. Dispatch is first-match-wins:
//
//	| Data shape                  | Path                                                       |
//	|-----------------------------|------------------------------------------------------------|
//	| [io.Reader]                 | [writeStream] — drained and hashed in-flight               |
//	| []byte                      | [writeBytes]                                               |
//	| string                      | []byte(s) → [writeBytes]                                   |
//	| interface{ Bytes() []byte } | v.Bytes() → [writeBytes]                                   |
//	| [encoding.BinaryMarshaler]  | MarshalBinary → [writeBytes]                               |
//	| [encoding.TextMarshaler]    | MarshalText → [writeBytes]                                 |
//
// Types that do not round-trip losslessly ([fmt.Stringer], [op.SourceConverter]) are rejected to prevent
// silent data loss. A nil data value is not handled here — callers short-circuit before calling.
//
// Parameters:
//   - root: filesystem root under which the parent directory is created and the file is written.
//   - sourcePath: destination path.
//   - data: payload value matching one of the dispatch rows above.
//
// Returns:
//   - string: SHA-256 of the written content, lowercase hex.
//   - error: parent-directory creation failure, unsupported data type, marshaler error, or write failure.
func writeSpecData(root op.Root, sourcePath op.Path, data any) (string, error) {

	parentRel := filepath.Dir(sourcePath.Rel())
	parentPath := root.NewPath(parentRel)
	if err := root.MkdirAll(parentPath, 0o700); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}

	switch v := data.(type) {

	case io.Reader:
		return writeStream(root, sourcePath, v)

	case []byte:
		return writeBytes(root, sourcePath, v)

	case string:
		return writeBytes(root, sourcePath, []byte(v))

	case interface{ Bytes() []byte }:
		return writeBytes(root, sourcePath, v.Bytes())

	case encoding.BinaryMarshaler:
		b, err := v.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("MarshalBinary: %w", err)
		}
		return writeBytes(root, sourcePath, b)

	case encoding.TextMarshaler:
		b, err := v.MarshalText()
		if err != nil {
			return "", fmt.Errorf("MarshalText: %w", err)
		}
		return writeBytes(root, sourcePath, b)

	default:
		return "", fmt.Errorf("unsupported Data type %T; want nil, io.Reader, []byte, string, "+
			"Bytes() []byte, encoding.BinaryMarshaler, or encoding.TextMarshaler", v)
	}
}

// writeStream drains reader into sourcePath, hashing in-flight with [io.TeeReader].
//
// Truncates any existing file at sourcePath. The file is closed via a deferred call; a close error replaces a nil err
// on return.
//
// Parameters:
//   - root: filesystem root under which the file is opened.
//   - sourcePath: destination path.
//   - reader: source of payload bytes; drained completely.
//
// Returns:
//   - string: SHA-256 of the streamed content, lowercase hex.
//   - error: open failure, copy failure, or close failure.
func writeStream(root op.Root, sourcePath op.Path, reader io.Reader) (_ string, err error) {

	f, err := root.OpenFile(sourcePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer iox.Close(&err, f)

	h := sha256.New()
	teed := io.TeeReader(reader, h)

	if _, err := io.Copy(f, teed); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
