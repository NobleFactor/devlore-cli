// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// entry represents a handle to a file on the disk identified by its path.
//
// entry carries identity only: the URI (derived from the absolute path) and the [fsroot.Path] handle.
// Runtime-observed state — size, mode, mod-time, inode, device, existence — lives on a separate [*Observation] minted
// by [Provider.Observe]; the framework owns observation storage so a buggy provider cannot corrupt the catalog by
// mutating fields on a shared [*resource] pointer.
type resource struct {
	op.ResourceBase

	// SourcePath is the bound location triad (§5.5): Rel() is the identity verbatim, Root() the bound
	// fsroot, Abs() derived and OS-native for I/O. Set at construction against the constructing session's
	// root; re-bound REL-FIRST to the run's root by [entry.BindRoot] at the executor's pre-flight
	// resolve pass (the op.RootBinder seam) and by [entry.Resolve].
	SourcePath fsroot.Path
}

// BindRoot re-binds this resource's location to `root`, rel-first — the activation binding (§5.5), driven
// from the executor's pre-flight resolve pass through the [op.RootBinder] seam.
//
// Identity (the rel) is unchanged; the root becomes the run's; the native form derives. The environment
// re-base happens executor-side, so after binding every observation — existence, Etag, Digest, I/O — reads
// the run's world.
//
// Parameters:
//   - `root`: the run's bound fsroot.
func (r *resource) BindRoot(root fsroot.Dir) {
	r.SourcePath = root.NewPath(r.SourcePath.Rel())
}

// discoverResource registers a catch-all base handle via [op.ResourceCatalog.Discover] without claiming production.
//
// SEALED (step 23, slice 4): the base has no public constructors — every public construction point mints a taxonomy
// variant, and the generator announces only exported constructors, so no coercion path can build a kindless
// resource. This unexported remnant serves the base's own rehydration methods and the package's base-behavior
// tests; it dies with the catch-all.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*resource`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, or the input violates RFC 8089 when in file URI form, or
//     [op.ResourceCatalog.Discover]'s strict assertions fail.
func discoverResource(runtimeEnvironment *op.RuntimeEnvironment, value any) (*resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*resource)
	if !ok {
		return nil, fmt.Errorf("file.discoverResource: catalog entry for %q is %T, want *file.entry",
			candidate.URI(),
			got)
	}

	return canonical, nil
}

// buildCandidate validates value, parses any file URI per RFC 8089, and constructs a [file.entry].
//
// This function does not touch the resource catalog. It is shared by [discoverResource] and internal helpers that
// need a base handle without interning. The body lives in [buildCandidateAs] — the variant-parameterized trunk the
// taxonomy constructors share (phase-8 step 23) — with the catch-all's own type id.
//
// Parameters:
//   - `runtimeEnvironment`: the session's runtime environment; supplies `Root` for path canonicalization and is
//     embedded via [op.NewResourceBase].
//   - `value`: an `any` carrying a string file path or file URI; other dynamic types are rejected.
//
// Returns:
//   - `*resource`: the constructed candidate. Not interned in the catalog — callers route it through
//     [op.ResourceCatalog] themselves.
//   - `error`: non-nil if `value` is not a string, the input violates RFC 8089 when in file URI form (non-file scheme,
//     userinfo, non-localhost host, query, fragment, or opaque form), or [op.NewResourceBase] fails.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (candidate *resource, err error) {

	return buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*resource]())
}

// region EXPORTED METHODS

// region State management

// Addressing reports that file.entry is location-keyed.
//
// Identity is the path on the disk, and bytes at that path are mutable. The catalog uses [op.AddressingLocation]
// semantics. Content drift triggers shadow chains, not new URIs.
//
// Returns:
//   - `AddressingMode`: always [op.AddressingLocation].
func (r *resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Digest returns the honest content hash: sha256 of the file's bytes, streamed (no full-file allocation).
//
// Always fresh: opens and reads the file at call time. Errors with [op.ErrUnimplemented] for directories: the base
// file.entry pre-dates the taxonomic split into Regular / Directory / SymbolicLink variants; directory hashing now
// lives on [Directory.Digest] (the Merkle root over the tree), so a directory is represented by a file.Directory.
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes.
//   - `error`: a stat error, [op.ErrUnimplemented] for directories, or any read error.
func (r *resource) Digest() (digest op.Digest, err error) {

	root := r.RuntimeEnvironment().Root()
	path := root.NewPath(r.SourcePath.Abs())

	var info fs.FileInfo

	info, err = root.Stat(path)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.resource: digest stat %s: %w", r.SourcePath.Abs(), err)
	}

	if info.IsDir() {
		return op.Digest{}, fmt.Errorf("file.resource: digest of directory %s: %w", r.SourcePath.Abs(), op.ErrUnimplemented)
	}

	var f fs.File

	f, err = root.Open(path)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.resource: digest open %s: %w", r.SourcePath.Abs(), err)
	}
	defer iox.Close(&err, f)

	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return op.Digest{}, fmt.Errorf("file.resource: digest read %s: %w", r.SourcePath.Abs(), err)
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// Equal reports whether `r` and `other` identify the same file resource.
//
// Strict equality: `other` must be a *file.entry (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal]. A cross-type URI collision (e.g., a file URI
// embedded in an appnet.Resource) fails at the type check rather than matching spuriously.
//
// Parameters:
//   - `other`: the value to compare against; may be `any`, including nil or a non-entry.
//
// Returns:
//   - `bool`: true if `other` is a *file.entry with the same URI as `r`.
func (r *resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns an inexpensive stat-derived change-detection token.
//
// Always fresh: stats the file at call time. The catalog uses Etag as an inexpensive signal that triggers a full
// [entry.Digest] comparison. It is a sha256 of (size, mtime_ns, ino) packed into a little-endian byte array encoded
// as a lowercase hex string.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple.
//   - `error`: any stat error (file gone, permission denied, etc.).
func (r *resource) Etag() (string, error) {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return "", fmt.Errorf("file.resource: etag stat %s: %w", r.SourcePath.Abs(), err)
	}

	return statTupleEtag(info), nil
}

// Exists reports whether the file exists on disk at the time of the call.
//
// Self-stat: performs a fresh stat at every call rather than reading any cached field. For richer metadata (size, mode,
// mod-time, etc.) call [Provider.Observe] which returns a [*Observation].
//
// Returns:
//   - `bool`: true when the file exists; false when the stat returns [os.ErrNotExist] or any other error.
func (r *resource) Exists() bool {
	root := r.RuntimeEnvironment().Root()
	_, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	return err == nil
}

// IsDir reports whether the file at this resource's path is a directory at the time of the call.
//
// Self-stat. Returns false for any stat error (not-exist, permission denied, etc.) — callers that need to distinguish
// "missing" from "not a directory" should call [Provider.Observe] and check `obs.Exists` and `obs.Mode.IsDir()`
// separately.
//
// Returns:
//   - `bool`: true when the file exists and is a directory; false otherwise.
func (r *resource) IsDir() bool {

	root := r.RuntimeEnvironment().Root()
	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))

	if err != nil {
		return false
	}

	return info.IsDir()
}

// Path returns the canonicalized absolute path handle on the disk.
//
// The [Resource] accessor: mixed-kind holders (an Resource from enumeration or a walker callback) reach the path without
// asserting a concrete variant. The handle is the construction-time [fsroot.Path]; [entry.Resolve] rebinds it to
// the live execution fsroot.
//
// Returns:
//   - `fsroot.Path`: the canonicalized absolute path handle.
func (r *resource) Path() fsroot.Path {
	return r.SourcePath
}

// String returns a debug-oriented single-line representation of the resource.
//
// Suitable for log lines and IDE debug windows. Identity-only — observation-shaped data (size, mode, mod-time) is not
// on the entry. Use [Provider.Observe] to capture observation values and log those alongside the entry when
// needed.
//
// Returns:
//   - `string`: `file.resource{uri=<URI>, source_path=<path>}`.
func (r *resource) String() string {
	return fmt.Sprintf("file.resource{uri=%s, source_path=%s}", r.URI(), r.SourcePath.Abs())
}

// endregion

// region Behaviors

// ConvertTo projects this file resource into the given target Go type — the string form is the PATH.
//
// Overrides [op.ResourceBase.ConvertTo], whose baseline yields the canonical tag URI: a file resource's reachable
// string form is its absolute path (step 23, ruling 2 — the string turn feeds provider path parameters, and
// `op.ActionPlanner.Plan`'s location-immediate conversion is documented as producing path strings). The canonical
// URI remains the serialized identity via [op.ResourceBase.MarshalText]; only live-value projection is path-form.
// The taxonomy variants inherit this projection by promotion (always invoked on live values, never nil probes).
//
// Parameters:
//   - `target`: the destination Go type the caller wants to project the resource into.
//
// Returns:
//   - `any`: the absolute source path (as a Go string) when `target` is string.
//   - `error`: non-nil if `target` is not a recognized conversion.
func (r *resource) ConvertTo(target reflect.Type) (any, error) {

	if target == reflect.TypeFor[string]() {
		return r.SourcePath.Abs(), nil
	}

	return r.ResourceBase.ConvertTo(target)
}

// CanConvertFrom reports whether `source` can be projected into a [*resource] via [entry.ConvertFrom].
//
// Opts the file entry into the framework's [op.TargetConverter] contract: the [op.Convert] cascade routes `source →
// *resource` slot-fill through [entry.ConvertFrom] at dispatch time (step 6 of the cascade), and
// [op.typesAreInterconvertible] consults the same probe at plan time so [op.Subgraph.mergeBubbled] does not flag a
// variable bound to both a `string` slot and a `*resource` slot as a collision. Today's accepted source shape is
// `string` — interpreted as a filesystem path under the active fsroot. Other source shapes (file URI strings, Path
// values) can be added by extending this probe; the conversion body in [entry.ConvertFrom] must accept the
// corresponding type.
//
// Cheap-probe contract: this method is called against a nil-or-zero `*resource` receiver by
// [op.typesAreInterconvertible] during plan-time bubble-up checks. It MUST NOT dereference receiver fields.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*resource) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into a fresh [*resource].
//
// Today's accepted shape is `string` — interpreted as a filesystem path under the active fsroot. The returned
// [*resource] carries the path under [entry.SourcePath] but is NOT catalog-interned at this layer; provider methods
// that receive the projected entry are responsible for interning via their own taxonomy constructor
// path. This mirrors the inline `&resource{SourcePath: fsroot.NewPath("", str)}` pattern used at writ adopt call sites
// pre-13.0(n) — the slot-fill cascade absorbs the pattern uniformly.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*resource].
//   - `error`: non-nil when `value` is not a `string`.
func (*resource) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.entry.ConvertFrom: source must be string, got %T", value)
	}

	return &resource{SourcePath: fsroot.NewPath("", str)}, nil
}

// Resolve rebinds the source path to the execution fsroot and verifies the file exists.
//
// The path is canonical from construction; rebinding updates Rel for confined I/O under the execution fsroot. If the
// file does not exist, Resolve returns nil — existence is observation, not identity, and `not-exist` is a valid
// observation outcome. Other stat failures (permission denied, I/O error) surface as errors.
//
// Resolve does not populate any observation-shaped metadata on the entry. Callers that need metadata call
// [Provider.Observe] to get an [Observation] value the framework can catalog.
//
// Returns:
//   - `error`: any stat error other than not-exist.
func (r *resource) Resolve() error {

	root := r.RuntimeEnvironment().Root()

	// Rel-first (§5.5): identity is the rel and location derives from the live root. The abs-first form
	// this replaces preserved the construction-time machine location — the run-from-elsewhere defect.
	r.SourcePath = root.NewPath(r.SourcePath.Rel())

	_, err := root.Stat(r.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat: %w", err)
	}

	return nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `data`: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := discoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a file path or file URI.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := discoverResource(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from a YAML scalar (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `unmarshal`: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the YAML node does not decode as a string, or resource
//     construction fails.
func (r *resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := discoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion
