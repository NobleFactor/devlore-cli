// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a cloned git repository.
//
// Identity is the local clone's filesystem location, stored as a file:// URI in [op.ResourceBase]. Every domain field —
// Ref, HEAD, Remotes, Bare, Dirty — is populated by [Resource.Resolve] from the on-disk `.git/` contents. Ref and HEAD
// are additionally persisted through JSON/YAML so a serialized Resource can carry its version snapshot to contexts
// where Resolve cannot run (e.g., cross-host comparison, offline inspection); Remotes, Bare, and Dirty are operational
// and not persisted — they're always rebuilt by Resolve.
type Resource struct {
	op.ResourceBase

	// SourcePath is the local clone's canonical absolute path; identity derives from this via the file:// URI. Not
	// persisted — reconstructed from the URI on deserialization.
	SourcePath op.Path `json:"-" yaml:"-"`

	// Ref is the branch, tag, or commit reference the clone is positioned at as plan-time intent.
	// Set at construction by [Provider.Clone] (from the just-cloned tree's `.git/HEAD`) or by
	// wire-form deserialization (from a saved plan). Not mutated by [Resource.Resolve]; the runtime
	// view of the disk's current ref lives on [Observation.ObservedRef].
	Ref string `json:"ref,omitempty" yaml:"ref,omitempty"`

	// HEAD is the commit SHA (40-char hex) the clone was positioned at as plan-time intent. Set at
	// construction by [Provider.Clone] (from the just-cloned tree's `.git/HEAD`) or by wire-form
	// deserialization. Pins the clone to an exact version across serialization. Empty for resources
	// constructed via [NewResource] without an associated clone. Not mutated by [Resource.Resolve];
	// the runtime view of the disk's current HEAD lives on [Observation.ObservedHEAD].
	HEAD string `json:"head,omitempty" yaml:"head,omitempty"`
}

// Remote carries the fetch and push URLs for a named git remote.
//
// PushURL is empty when the push direction uses FetchURL (git's default) — the distinction matters for workflows that
// split read and write endpoints (e.g., HTTPS fetch / SSH push, mirror fetch / authoritative push).
type Remote struct {
	FetchURL string
	PushURL  string
}

// NewResource constructs a git.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped
// with `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles,
// scanner-style discovery).
//
// The input may be a bare filesystem path ("/opt/repo") or a file URI ("file:///opt/repo"). File URIs are
// strictly validated per RFC 8089 — userinfo, non-localhost host, query, fragment, and opaque form are
// rejected. Identity is the canonical file:// URI computed from the resolved absolute path; remotes, ref,
// HEAD, and other metadata are populated post-construction by Clone, Resolve, or explicit setters.
//
// Nil-Catalog tolerance mirrors [DiscoverResource]: when `activationRecord.RuntimeEnvironment.Catalog` is nil
// (test fixtures, library callers without a runtime), the candidate is returned unlinked.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `RuntimeEnvironment` carries the runtime
//     environment and its `Unit.ID()` becomes the catalog entry's producerID (empty when `Unit` is nil).
//     Must be non-nil.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, or the input violates RFC 8089 when in file URI form, or
//     [op.ResourceCatalog.GetOrCreate]'s strict assertions fail.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.GetOrCreate(activationRecord, candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("git.NewResource: catalog entry for %q is %T, want *git.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource registers a git.Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Use DiscoverResource from non-production callsites: receipt rehydration (UnmarshalJSON/Text/YAML),
// reference handles in CLI tools, and scanner-style discovery walks. The returned catalog entry has no
// producer stamp (or carries whatever stamp a previous [NewResource] call already applied). Use
// [NewResource] instead when the caller is a producer claiming this Resource as its output.
//
// `activationRecord` is required for signature consistency with [NewResource], but only its
// `RuntimeEnvironment` is consumed — `Unit` is unused since Discover doesn't stamp a producer. Discovery
// callers commonly construct one as `op.NewActivationRecord(nil, nil, ctx)` — both `Graph` and `Unit` nil.
//
// Nil-Catalog tolerance mirrors the receipt-rehydration paths: when `activationRecord.RuntimeEnvironment.Catalog`
// is nil, the candidate is returned unlinked.
//
// Parameters:
//   - `activationRecord`: provides the runtime environment via `activationRecord.RuntimeEnvironment`. `Unit` is
//     unused. Must be non-nil.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, or the input violates RFC 8089 when in file URI form.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("git.DiscoverResource: catalog entry for %q is %T, want *git.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate validates value and constructs a *Resource without touching the catalog.
//
// Shared by [NewResource] and [DiscoverResource].
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment threaded into the produced [op.ResourceBase].
//   - `value`: a string file path or file URI; any other type is an error.
//
// Returns:
//   - *Resource: the candidate, not yet interned in the catalog.
//   - `error`: if `value` is not a string, or violates RFC 8089 when in file URI form.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("git.Resource: expected string, got %T", value)
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("git.Resource: invalid input %q: %w", path, err)
	}

	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return nil, fmt.Errorf("git.Resource: expected file scheme, got %q in %q", parsed.Scheme, path)
	}

	if parsed.Scheme == "file" {

		if parsed.User != nil {
			return nil, fmt.Errorf("git.Resource: userinfo not permitted in %q", path)
		}

		if parsed.Host != "" && parsed.Host != "localhost" {
			return nil, fmt.Errorf("git.Resource: unexpected host %q in %q", parsed.Host, path)
		}

		if parsed.RawQuery != "" {
			return nil, fmt.Errorf("git.Resource: query not permitted in %q", path)
		}

		if parsed.Fragment != "" {
			return nil, fmt.Errorf("git.Resource: fragment not permitted in %q", path)
		}

		if parsed.Opaque != "" {
			return nil, fmt.Errorf("git.Resource: opaque form not permitted in %q; use file:///path", path)
		}

		path = parsed.Path
	}

	sourcePath := runtimeEnvironment.Root.NewPath(path)

	base, err := op.NewResourceBase(runtimeEnvironment, "file://"+sourcePath.Abs(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourcePath:   sourcePath,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that git.Resource is location-keyed.
//
// The identity is the local clone's filesystem location, and the bytes under that location (commit SHAs, working-tree
// contents) are mutable.
//
// The catalog uses [op.AddressingLocation]
// semantics — content drift triggers shadow chains, not new URIs.
//
// Returns:
//   - op.AddressingMode: always [op.AddressingLocation].
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Digest returns the honest content hash for the local clone:
//
//   - Clean repository (bare or working-tree): sha256 of HEAD's hex string.
//   - Dirty working tree: sha256 of HEAD + "\n" + tree SHA over the index + working tree.
//
// The HEAD SHA-1 itself already content-addresses git's commit graph; wrapping it in a sha256 layer keeps the algorithm
// consistent with the rest of the system (the catalog stores `op.Digest` values uniformly and round trips them through
// [op.ParseDigest], which only accepts the sha256 allowlist). For dirty working trees, the tree SHA (derived from
// stash-create followed by rev-parse to the tree, not the commit SHA which would carry timestamps) captures the index +
// working-tree state deterministically — same state same digest.
//
// Always fresh — recomputes at call time. Errors when the path is not a git repository or HEAD cannot be read.
//
// Returns:
//   - op.Digest: sha256 of the HEAD SHA (plus stash-create tree SHA when the working tree is dirty).
//   - `error`: when the path is not a git repository or HEAD cannot be read.
func (r *Resource) Digest() (op.Digest, error) {

	abs := r.SourcePath.Abs()

	repo, bare := isGitRepo(abs)
	if !repo {
		return op.Digest{}, fmt.Errorf("git.Resource: digest: %s is not a git repository", abs)
	}

	head := readHEADSha(abs)
	if head == "" {
		return op.Digest{}, fmt.Errorf("git.Resource: digest: cannot read HEAD at %s", abs)
	}

	h := sha256.New()
	h.Write([]byte(head))

	if !bare {
		stashID := readStashCreateID(abs)
		if stashID != "" {
			h.Write([]byte("\n"))
			h.Write([]byte(stashID))
		}
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// Equal reports whether r and other identify the same git resource.
//
// Strict equality: other must be a *git.Resource (not merely an [op.Resource] with the same URI). Once the type check
// passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - `bool`: true if `other` is a *git.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns a cheap stat-derived change-detection token for the local clone:
//
//   - Bare repository: the 7-character HEAD short-id (e.g., "a1b2c3d").
//   - Working tree, clean: the 7-character HEAD short-id.
//   - Working tree, dirty: HEAD short-id + "-" + 7-character prefix of the tree SHA covering the current index +
//     working tree.
//
// The dirty fingerprint is derived from `git stash create` followed by `git rev-parse <stash>^{tree}`. The stash
// commit's own SHA cannot be used directly: commit objects include author/committer timestamps, so two calls on the
// same unchanged tree state would produce different commit SHAs (catalog would falsely detect drift on every Resolve).
// The tree SHA is content-addressed and timestamp-free — same tree state same SHA, different tree state different SHA.
// This lets the catalog detect drift within the dirty state without false-positive drift on identical state.
//
// Always fresh — re-reads HEAD and (when dirty) re-runs the stash-create + rev-parse pair at call time. Errors when the
// path is not a git repository or HEAD cannot be read.
//
// Returns:
//   - `string`: the etag (HEAD short-id, optionally suffixed with `-<tree-short>` for a dirty working tree).
//   - `error`: when the path is not a git repository or HEAD cannot be read.
func (r *Resource) Etag() (string, error) {

	abs := r.SourcePath.Abs()

	repo, bare := isGitRepo(abs)
	if !repo {
		return "", fmt.Errorf("git.Resource: etag: %s is not a git repository", abs)
	}

	head := readHEADSha(abs)
	if head == "" {
		return "", fmt.Errorf("git.Resource: etag: cannot read HEAD at %s", abs)
	}

	short := head
	if len(short) > 7 {
		short = short[:7]
	}

	if bare {
		return short, nil
	}

	stashID := readStashCreateID(abs)
	if stashID == "" {
		return short, nil
	}

	suffix := stashID
	if len(suffix) > 7 {
		suffix = suffix[:7]
	}

	return short + "-" + suffix, nil
}

// String returns a debug-oriented single-line representation of the resource.
//
// Suitable for log lines and debug windows. Identity-only — runtime-observed state (bare, dirty,
// remotes, disk's current HEAD/ref) lives on [*Observation], minted by [Provider.Observe].
//
// Returns:
//   - `string`: `git.Resource{uri=<URI>, ref=<ref>, head=<head>}`.
func (r *Resource) String() string {
	return fmt.Sprintf("git.Resource{uri=%s, ref=%s, head=%s}", r.URI(), r.Ref, r.HEAD)
}

// endregion

// region Behaviors

// Resolve rebinds the source path to the execution root and verifies the path is reachable.
//
// Existence-check only — no field mutation. Runtime observation of the on-disk clone (Bare, Dirty,
// Remotes, the disk's current HEAD/ref) flows through [Provider.Observe], which returns a
// [*Observation] that the framework can catalog independently of this Resource.
//
// A path that does not exist, is not a directory, or is not a git repository is not an error here
// — those are observable conditions, not identity-level failures. Callers that need the distinction
// call [Provider.Observe] and inspect the returned observation.
//
// Returns:
//   - `error`: currently always nil; reserved for future identity-level surfacing.
func (r *Resource) Resolve() error {

	root := r.RuntimeEnvironment().Root
	r.SourcePath = root.NewPath(r.SourcePath.Abs())

	return nil
}

// CanConvertFrom reports whether `source` can be projected into a [*Resource] via [Resource.ConvertFrom].
//
// Opts the git Resource into the framework's [op.TargetConverter] contract — accepted source shape is `string`
// (interpreted as a local clone's filesystem path or a git URL). The framework consults this probe both at
// plan-time via [op.typesAreInterconvertible] (the bubble-up parameter-consistency check honors the
// convertibility relation without running an actual conversion) and at dispatch-time via [op.Convert] step 7
// (env-less fallback). The canonical dispatch-time path remains the registered constructor at [op.Convert]
// step 6, which receives the full [op.RuntimeEnvironment] and produces a fully-canonicalized Resource via
// [buildCandidate].
//
// Cheap-probe contract: this method is called against a nil-or-zero `*Resource` receiver by
// [op.typesAreInterconvertible] during plan-time bubble-up checks. MUST NOT dereference receiver fields.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*Resource) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into an env-less unlinked [*Resource].
//
// Used by [op.Convert] step 7 when the env-aware registered constructor (step 6) is unavailable — env-less
// library callers, tests, or [op.RuntimeEnvironment.Registry]-missing contexts. The returned Resource carries
// only the SourcePath set from `value`; URI / Ref / HEAD / catalog interning are not populated here. Provider
// methods consuming the projected Resource are responsible for re-canonicalization via their own
// [NewResource]/[DiscoverResource] path when full identity is required.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*Resource].
//   - `error`: non-nil when `value` is not a `string`.
func (*Resource) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("git.Resource.ConvertFrom: source must be string, got %T", value)
	}

	return &Resource{SourcePath: op.NewPath("", str)}, nil
}

// UnmarshalJSON populates the receiver from its JSON wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method. Identity is reconstructed via [NewResource] from the URI; Ref and HEAD are assigned from the decoded
// snapshot. Operational state (Remotes, Bare, Dirty) stays at zero values until [Resource.Resolve] reads the on-disk
// clone.
//
// Parameters:
//   - `data`: JSON-encoded wire form.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode, or resource construction fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("git.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	type alias Resource
	aux := &struct {
		URI string `json:"uri"`
		*alias
	}{alias: (*alias)(r)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	ref, head := r.Ref, r.HEAD

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), aux.URI)
	if err != nil {
		return err
	}

	built.Ref = ref
	built.HEAD = head

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a local path or file URI.
//
// Scalar form: only identity (URI) round-trips. Ref, HEAD, and Remotes remain at zero values; richer round trip uses
// [Resource.UnmarshalJSON] or [Resource.UnmarshalYAML].
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("git.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method. Identity is reconstructed via [NewResource] from the URI; Ref and HEAD are assigned from the decoded
// snapshot. Operational state (Remotes, Bare, Dirty) stays at zero values until [Resource.Resolve] reads the on-disk
// clone.
//
// Parameters:
//   - `unmarshal`: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the YAML does not decode, or resource construction fails.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("git.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	type alias Resource
	aux := &struct {
		URI string `yaml:"uri"`
		*alias
	}{alias: (*alias)(r)}

	if err := unmarshal(aux); err != nil {
		return err
	}

	ref, head := r.Ref, r.HEAD

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), aux.URI)
	if err != nil {
		return err
	}

	built.Ref = ref
	built.HEAD = head

	*r = *built
	return nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// readStashCreateID returns a deterministic tree SHA over the index + working-tree state at `path`.
//
// Two-step: `git stash create` constructs a stash commit object covering both the index and working tree without
// actually stashing; `git rev-parse <stash>^{tree}` then projects to the tree SHA. The intermediate stash commit's own
// SHA cannot be used directly — commit objects include author/committer timestamps, so two calls on the same unchanged
// tree state would produce different commit SHAs (catalog would falsely detect drift on every Resolve). Tree SHAs are
// content-addressed and timestamp-free: same tree state same SHA, different tree state different SHA, regardless of
// when the call runs.
//
// Untracked files are not included by stash-create's default scope; callers that need untracked-file
// fingerprinting must add it separately.
//
// Parameters:
//   - `path`: filesystem path to the git working tree to fingerprint.
//
// Returns:
//   - `string`: the deterministic tree SHA covering the index plus working tree, or "" when clean, not
//     a working tree, or the underlying git command fails.
func readStashCreateID(path string) string {

	stash := runGitOutput(path, "stash", "create")
	if stash == "" {
		return ""
	}

	return runGitOutput(path, "rev-parse", stash+"^{tree}")
}

// runGitOutput runs `git -C path <args...>` and returns the trimmed stdout, or "" on any error.
//
// Parameters:
//   - `path`: filesystem path passed to git via `-C`.
//   - `args`: the remaining git subcommand and its flags.
//
// Returns:
//   - `string`: trimmed stdout on success, or "" on any error (including non-zero exit).
func runGitOutput(path string, args ...string) string {

	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// endregion

// endregion
