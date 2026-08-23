// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Directory is the taxonomy variant asserting that its path names a directory (phase-8 step 23).
//
// The kind is declared intent, never stat-assigned (ruling 1): planning is offline, so the assertion is verified at
// use rather than at construction — [Directory.Digest] and [Directory.Etag] observe the disk with lstat semantics
// and error with a kind mismatch when the entry is anything else (ruling 5e). Identity is the embedded [entry]
// (URI + SourcePath); runtime-observed metadata lives on [*Observation], exactly as for the base.
type Directory struct {
	entry
}

// sealedEntry marks Directory as a member of the closed [Entry] set (step 23, slice 4).
func (*Directory) sealedEntry() {}

// Exists reports whether a DIRECTORY exists at this resource's path — lstat plus kind test (kind-honest
// activation, ruled 2026-08-22; step 23 ruling 5e).
//
// Kinds are lstat-strict: a regular file or a symbolic link at the path is not this resource, so a
// *Directory claim over one fails verification at the starting line — "claims are true when made" —
// rather than activating kind-blind and failing later at observation or I/O.
//
// Returns:
//   - `bool`: true when the path holds a directory; false on any lstat error or any other kind.
func (r *Directory) Exists() bool {

	root := r.RuntimeEnvironment().Root()
	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	return err == nil && info.Mode().IsDir()
}

// NewDirectory constructs a [file.Directory] and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewDirectory from a producer dispatch context; the returned Directory is the
// canonical catalog entry, stamped with the given `producerID` when non-empty. A catalog entry already
// claimed under a different kind for the same URI is an error — cross-kind plan conflicts surface at the earliest
// moment. Nil-Catalog tolerance: the candidate is returned unlinked when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `producerID`: the producing caller's id (`activationRecord.CallerID` — a unit id under graph dispatch, a
//     starlark call-site under script dispatch), or "" for caller-less dispatch (an empty producer stamp).
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*Directory`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func NewDirectory(runtimeEnvironment *op.RuntimeEnvironment, producerID string, value any) (*Directory, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*Directory]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, producerID, true, &Directory{entry: *base})
}

// DiscoverDirectory registers a [file.Directory] via [op.ResourceCatalog.Discover] without claiming production.
//
// The discovery counterpart of [NewDirectory]: no producer is stamped, so no unit
// reference is taken. Nil-Catalog tolerance returns the unlinked candidate.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*Directory`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func DiscoverDirectory(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Directory, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*Directory]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, "", false, &Directory{entry: *base})
}

// region EXPORTED METHODS

// region State management

// Digest returns the Merkle root of the directory tree (phase-8 step 23 — the chartered scheme).
//
// Always fresh: the disk is observed at call time. Each directory's digest is a sha256 over its immediate entries in
// byte-wise lexicographic name order (the fs.ReadDir guarantee — platform-stable, ruling 5c), each entry contributing
// an unambiguous record: one kind marker byte ('f' regular file, 'd' directory, 'l' symlink), the entry name, a NUL
// delimiter, and the entry's 32-byte digest. A regular file digests by content (streamed sha256); a symlink digests
// by the sha256 of its literal readlink target, never following (matching ruling 5a); a subdirectory digests by its
// own Merkle root, recursively. Entry names carry no path separators, so the serialization is identical on every
// platform, and only the tree's own shape and content participate — the enclosing absolute path does not.
//
// The root covers everything (ruling 5d): no gitignore filtering and no `.git` skip — a digest that skips content
// would report "unmodified" over a modified tree. The empty directory digests deterministically (the hash over zero
// entries). An entry of any other kind (FIFO, socket, device) is an error: a digest cannot honestly identify what it
// cannot hash. The entry itself must be a directory — the kind check uses lstat semantics, and any other observed
// kind errors with a kind mismatch (ruling 5e).
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes — the Merkle root.
//   - `error`: an lstat error, a kind mismatch, an unsupported entry kind, or any read error during the walk.
func (r *Directory) Digest() (op.Digest, error) {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Directory: digest lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if !info.IsDir() {
		return op.Digest{}, kindMismatchError("file.Directory", r.SourcePath.Abs(), info.Mode())
	}

	return merkleRoot(root, r.SourcePath.Abs())
}

// Equal reports whether `r` and `other` identify the same directory resource.
//
// Strict equality mirroring [entry.Equal]: `other` must be a *file.Directory — the same URI held by another kind
// (or by the catch-all base) does not match. Once the type check passes, URI comparison is delegated to
// [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: the value to compare against; may be `any`, including nil or a non-Directory.
//
// Returns:
//   - `bool`: true if `other` is a *file.Directory with the same URI as `r`.
func (r *Directory) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Directory); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns the inexpensive stat-derived change-detection token for the directory.
//
// The cheap counterpart of the Merkle-root [Directory.Digest] (the chartered pairing): the disk is observed at call
// time with lstat semantics, a kind other than directory errors with a kind mismatch (step 23, ruling 5e), and the
// token is the shared stat-tuple form: a sha256 of (size, mtime_ns, ino) packed little-endian, encoded as lowercase
// hex. A directory's mtime moves on immediate-child creation, deletion, and rename, so the Etag is a shallow signal:
// the catalog treats a changed Etag as the trigger for the full Digest comparison, exactly as for regular files.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple.
//   - `error`: an lstat error or a kind mismatch.
func (r *Directory) Etag() (string, error) {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return "", fmt.Errorf("file.Directory: etag lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if !info.IsDir() {
		return "", kindMismatchError("file.Directory", r.SourcePath.Abs(), info.Mode())
	}

	return statTupleEtag(info), nil
}

// String returns a debug-oriented single-line representation of the directory resource.
//
// Returns:
//   - `string`: `file.Directory{uri=<URI>, source_path=<path>}`.
func (r *Directory) String() string {
	return fmt.Sprintf("file.Directory{uri=%s, source_path=%s}", r.URI(), r.SourcePath.Abs())
}

// endregion

// region Behaviors

// CanConvertFrom reports whether `source` can be projected into a [*Directory] via [Directory.ConvertFrom].
//
// The variant's own probe for the framework's [op.TargetConverter] contract — defined directly (not promoted from
// the embedded base) because the cheap-probe contract calls it against a nil-or-zero `*Directory` receiver, and a
// promoted method would dereference the nil receiver to reach the embedded base. Today's accepted source shape is
// `string`, interpreted as a filesystem path under the active fsroot.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*Directory) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into a fresh [*Directory].
//
// Mirrors [entry.ConvertFrom]: the returned value carries the path under SourcePath but is NOT catalog-interned
// at this layer; receiving provider methods intern via their own [NewDirectory]/[DiscoverDirectory] path.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*Directory].
//   - `error`: non-nil when `value` is not a `string`.
func (*Directory) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Directory.ConvertFrom: source must be string, got %T", value)
	}

	return &Directory{entry: entry{SourcePath: fsroot.NewPath("", str)}}, nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant — defined directly so
// rehydration rebuilds a [*Directory], never a half-filled embedded base.
//
// Parameters:
//   - `data`: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *Directory) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Directory: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverDirectory(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a file path or file URI.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *Directory) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Directory: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverDirectory(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from a YAML scalar (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant.
//
// Parameters:
//   - `unmarshal`: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the YAML node does not decode as a string, or resource
//     construction fails.
func (r *Directory) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Directory: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverDirectory(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// merkleRoot computes the Merkle root of the directory tree at `absDir` (see [Directory.Digest] for the scheme).
//
// Each level reads its immediate entries via fs.ReadDir — whose byte-wise lexicographic name order is the scheme's
// platform-stable sort — and folds one record per entry into a sha256: the kind marker byte, the name, a NUL
// delimiter, and the entry's 32-byte digest (content hash for a regular file, literal-target hash for a symlink, the
// recursive Merkle root for a subdirectory). Zero entries fold to the hash of empty input, making the empty
// directory deterministic.
//
// Parameters:
//   - `root`: the [fsroot.Dir] the walk is confined to.
//   - `absDir`: the absolute path of the directory to digest.
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes — the Merkle root of the tree.
//   - `error`: a read failure anywhere in the walk, or an entry of an unsupported kind (FIFO, socket, device).
func merkleRoot(root fsroot.Dir, absDir string) (op.Digest, error) {

	relDir := root.NewPath(absDir).Rel()
	if relDir == "" {
		relDir = "."
	}

	entries, err := fs.ReadDir(root.FS(), relDir)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Directory: merkle read %s: %w", absDir, err)
	}

	h := sha256.New()

	for _, entry := range entries {

		entryAbs := filepath.Join(absDir, entry.Name())

		var kind byte
		var entryDigest []byte

		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			kind = 'l'
			target, err := root.Readlink(root.NewPath(entryAbs))
			if err != nil {
				return op.Digest{}, fmt.Errorf("file.Directory: merkle readlink %s: %w", entryAbs, err)
			}
			sum := sha256.Sum256([]byte(target))
			entryDigest = sum[:]

		case entry.IsDir():
			kind = 'd'
			sub, err := merkleRoot(root, entryAbs)
			if err != nil {
				return op.Digest{}, err
			}
			entryDigest = sub.Bytes

		case entry.Type().IsRegular():
			kind = 'f'
			sub, err := contentDigest(root, entryAbs)
			if err != nil {
				return op.Digest{}, fmt.Errorf("file.Directory: merkle read %s: %w", entryAbs, err)
			}
			entryDigest = sub.Bytes

		default:
			return op.Digest{}, fmt.Errorf(
				"file.Directory: merkle digest of %s: unsupported entry kind %s", entryAbs, entry.Type())
		}

		h.Write([]byte{kind})
		h.Write([]byte(entry.Name()))
		h.Write([]byte{0})
		h.Write(entryDigest)
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}
