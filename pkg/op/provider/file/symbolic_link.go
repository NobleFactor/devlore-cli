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

// SymbolicLink is the taxonomy variant asserting that its path names a symbolic link (phase-8 step 23).
//
// The kind is declared intent, never stat-assigned (ruling 1): planning is offline, so the assertion is verified at
// use rather than at construction — [SymbolicLink.Digest] and [SymbolicLink.Etag] observe the disk with lstat
// semantics and error with a kind mismatch when the entry is anything else (ruling 5e). A dangling link is legal
// everywhere: the link is the resource, not its referent, which has its own resource identity. Identity is the
// embedded [Resource] (URI + SourcePath); runtime-observed metadata lives on [*Observation].
type SymbolicLink struct {
	Resource
}

// sealedEntry marks SymbolicLink as a member of the closed [Entry] set (step 23, slice 4).
func (*SymbolicLink) sealedEntry() {}

// NewSymbolicLink constructs a [file.SymbolicLink] and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewSymbolicLink from a producer dispatch context; the returned SymbolicLink is the
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
//   - `*SymbolicLink`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func NewSymbolicLink(
	runtimeEnvironment *op.RuntimeEnvironment,
	producerID string,
	value any,
) (*SymbolicLink, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*SymbolicLink]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, producerID, true, &SymbolicLink{Resource: *base})
}

// DiscoverSymbolicLink registers a [file.SymbolicLink] via [op.ResourceCatalog.Discover] without claiming production.
//
// The discovery counterpart of [NewSymbolicLink]: no producer is stamped, so no unit
// reference is taken. Nil-Catalog tolerance returns the unlinked candidate.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*SymbolicLink`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func DiscoverSymbolicLink(runtimeEnvironment *op.RuntimeEnvironment, value any) (*SymbolicLink, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*SymbolicLink]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, "", false, &SymbolicLink{Resource: *base})
}

// region EXPORTED METHODS

// region State management

// Digest returns the honest content hash of the link itself: sha256 of its target in canonical slash form,
// never following.
//
// A symbolic link IS a tiny file whose content is a path — hashing that content is the honest digest (step 23,
// ruling 5a). The target is taken from readlink with no cleaning and no absolutization — only separator
// canonicalization to slash form, because Windows reads a created link back with native separators and equal
// logical targets must digest equally on every platform (the same rule [fsroot.Path]'s Rel follows for document
// bytes; #556). A dangling link digests normally, and no cycle is possible because nothing is followed. The
// entry itself must be a symbolic link — any other observed kind errors with a kind mismatch (ruling 5e).
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes — the hash of the literal target path.
//   - `error`: an lstat error, a kind mismatch, or a readlink failure.
func (r *SymbolicLink) Digest() (op.Digest, error) {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.SymbolicLink: digest lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if info.Mode()&fs.ModeSymlink == 0 {
		return op.Digest{}, kindMismatchError("file.SymbolicLink", r.SourcePath.Abs(), info.Mode())
	}

	target, err := root.Readlink(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.SymbolicLink: digest readlink %s: %w", r.SourcePath.Abs(), err)
	}

	sum := sha256.Sum256([]byte(filepath.ToSlash(target)))
	return op.Digest{Algorithm: "sha256", Bytes: sum[:]}, nil
}

// Equal reports whether `r` and `other` identify the same symbolic-link resource.
//
// Strict equality mirroring [Resource.Equal]: `other` must be a *file.SymbolicLink — the same URI held by another
// kind (or by the catch-all base) does not match. Once the type check passes, URI comparison is delegated to
// [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: the value to compare against; may be `any`, including nil or a non-SymbolicLink.
//
// Returns:
//   - `bool`: true if `other` is a *file.SymbolicLink with the same URI as `r`.
func (r *SymbolicLink) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*SymbolicLink); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns the inexpensive stat-derived change-detection token for the link inode itself.
//
// Lstat-based (step 23, ruling 5b): the token reflects the link, not its referent, so a dangling link has a valid
// Etag. This fixes by construction the catch-all's latent defect — its Etag stats through `root.Stat`, which FOLLOWS
// symlinks, so a link's token reflected its referent and errored on a dangling link. A kind other than symbolic link
// errors with a kind mismatch (ruling 5e). The token is the shared stat-tuple form: a sha256 of (size, mtime_ns,
// ino) packed little-endian, encoded as lowercase hex.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple of the link inode.
//   - `error`: an lstat error or a kind mismatch.
func (r *SymbolicLink) Etag() (string, error) {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return "", fmt.Errorf("file.SymbolicLink: etag lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if info.Mode()&fs.ModeSymlink == 0 {
		return "", kindMismatchError("file.SymbolicLink", r.SourcePath.Abs(), info.Mode())
	}

	return statTupleEtag(info), nil
}

// String returns a debug-oriented single-line representation of the symbolic-link resource.
//
// Returns:
//   - `string`: `file.SymbolicLink{uri=<URI>, source_path=<path>}`.
func (r *SymbolicLink) String() string {
	return fmt.Sprintf("file.SymbolicLink{uri=%s, source_path=%s}", r.URI(), r.SourcePath.Abs())
}

// endregion

// region Behaviors

// CanConvertFrom reports whether `source` can be projected into a [*SymbolicLink] via [SymbolicLink.ConvertFrom].
//
// The variant's own probe for the framework's [op.TargetConverter] contract — defined directly (not promoted from
// the embedded base) because the cheap-probe contract calls it against a nil-or-zero `*SymbolicLink` receiver, and a
// promoted method would dereference the nil receiver to reach the embedded base. Today's accepted source shape is
// `string`, interpreted as a filesystem path under the active fsroot.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*SymbolicLink) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into a fresh [*SymbolicLink].
//
// Mirrors [Resource.ConvertFrom]: the returned value carries the path under SourcePath but is NOT catalog-interned
// at this layer; receiving provider methods intern via their own [NewSymbolicLink]/[DiscoverSymbolicLink] path.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*SymbolicLink].
//   - `error`: non-nil when `value` is not a `string`.
func (*SymbolicLink) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.SymbolicLink.ConvertFrom: source must be string, got %T", value)
	}

	return &SymbolicLink{Resource: Resource{SourcePath: fsroot.NewPath("", str)}}, nil
}

// Resolve rebinds the source path to the execution fsroot and verifies the link itself exists.
//
// Shadows [Resource.Resolve], whose existence check goes through [fsroot.Dir]'s Stat and therefore FOLLOWS the
// link — an escaping or absolute target would turn the check into the kernel's containment refusal even though
// the link itself landed exactly as asked (#556). The link is the resource, not its referent (ruling 5b), so the
// check here is lstat: a dangling or escaping target is a legal on-disk state, and any follow is judged by the
// kernel at use.
//
// Returns:
//   - `error`: any lstat error other than not-exist.
func (r *SymbolicLink) Resolve() error {

	root := r.RuntimeEnvironment().Root()

	r.SourcePath = root.NewPath(r.SourcePath.Abs())

	if _, err := root.Lstat(r.SourcePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("file.SymbolicLink: resolve lstat %s: %w", r.SourcePath.Abs(), err)
	}

	return nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant — defined directly so
// rehydration rebuilds a [*SymbolicLink], never a half-filled embedded base.
//
// Parameters:
//   - `data`: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *SymbolicLink) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.SymbolicLink: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverSymbolicLink(r.RuntimeEnvironment(), uri)
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
func (r *SymbolicLink) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.SymbolicLink: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverSymbolicLink(r.RuntimeEnvironment(), string(text))
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
func (r *SymbolicLink) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.SymbolicLink: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverSymbolicLink(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion
