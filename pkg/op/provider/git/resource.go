// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a cloned git repository.
//
// Identity is the local clone's filesystem location, stored as a file:// URI in [op.ResourceBase]. Every domain
// field — Ref, HEAD, Remotes, Bare, Dirty — is populated by [Resource.Resolve] from the on-disk `.git/`
// contents. Ref and HEAD are additionally persisted through JSON/YAML so a serialized Resource can carry its
// version snapshot to contexts where Resolve cannot run (e.g., cross-host comparison, offline inspection);
// Remotes, Bare, and Dirty are operational and not persisted — they're always rebuilt by Resolve.
type Resource struct {
	op.ResourceBase

	// SourcePath is the local clone's canonical absolute path; identity derives from this via the file:// URI.
	// Not persisted — reconstructed from the URI on deserialization.
	SourcePath op.Path `json:"-" yaml:"-"`

	// Ref is the branch, tag, or commit reference the clone is positioned at. Populated by [Resource.Resolve]
	// from `.git/HEAD`; persisted for round-trip continuity when Resolve cannot be called.
	Ref string `json:"ref,omitempty" yaml:"ref,omitempty"`

	// HEAD is the resolved commit SHA (40-char hex) the clone is currently pointing to. Populated by
	// [Resource.Resolve] from `.git/HEAD`; persisted for round-trip continuity — pins the clone to an exact
	// version across serialization. Empty when unresolved.
	HEAD string `json:"head,omitempty" yaml:"head,omitempty"`

	// Remotes maps remote name (e.g., "origin") to its fetch/push URL pair. Populated by [Resource.Resolve]
	// from `.git/config`; not persisted.
	Remotes map[string]Remote `json:"-" yaml:"-"`

	// Bare reports whether this is a bare repository (no working tree). Populated by [Resource.Resolve];
	// not persisted.
	Bare bool `json:"-" yaml:"-"`

	// Dirty reports whether the working tree has uncommitted changes. Populated by [Resource.Resolve];
	// not persisted.
	Dirty bool `json:"-" yaml:"-"`
}

// NewResource constructs a git.Resource from a string path or a file URI.
//
// The input may be a bare filesystem path ("/opt/repo") or a file URI ("file:///opt/repo"). File URIs are
// strictly validated per RFC 8089 — userinfo, non-localhost host, query, fragment, and opaque form are rejected.
// Identity is the canonical file:// URI computed from the resolved absolute path; remotes, ref, HEAD, and other
// metadata are populated post-construction by Clone, Resolve, or explicit setters.
//
// Parameters:
//   - ctx: execution context.
//   - value: a string file path or file URI.
//
// Returns:
//   - *Resource: the initialized resource with URI and SourcePath set; all other fields zero.
//   - error: if value is not a string, or the input violates RFC 8089 when in file URI form.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

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

	sourcePath := ctx.Root.NewPath(path)

	base, err := op.NewResourceBase(ctx, "file://"+sourcePath.Abs(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourcePath:   sourcePath,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// Equal reports whether r and other identify the same git resource.
//
// Strict equality: other must be a *git.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - other: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - bool: true if other is a *git.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// MarshalJSON marshals the resource to its JSON wire form — identity (URI) plus snapshot markers (Ref, HEAD).
//
// Remotes, Bare, and Dirty are intentionally omitted; they are operational state re-derived by
// [Resource.Resolve] after the receiver has been rehydrated.
//
// Returns:
//   - []byte: JSON-encoded object.
//   - error: any error from [json.Marshal]; none under normal conditions.
func (r *Resource) MarshalJSON() ([]byte, error) {

	type alias Resource
	return json.Marshal(&struct {
		URI string `json:"uri"`
		*alias
	}{
		URI:   r.URI(),
		alias: (*alias)(r),
	})
}

// MarshalYAML marshals the resource to its YAML wire form — identity (URI) plus snapshot markers (Ref, HEAD).
//
// Returns:
//   - any: the value for the YAML encoder to serialize.
//   - error: nil under normal conditions.
func (r *Resource) MarshalYAML() (any, error) {

	type alias Resource
	return &struct {
		URI string `yaml:"uri"`
		*alias
	}{
		URI:   r.URI(),
		alias: (*alias)(r),
	}, nil
}

// Resolve inspects the local filesystem and populates operational metadata on the receiver.
//
// Rebinds SourcePath to the scoped execution root, then populates every derived field from the on-disk
// `.git/` contents: Bare (via [isGitRepo]), HEAD (via [readHEADSha]), Ref (via [readBranchName]; empty on
// detached HEAD), Dirty (via [isDirtyRepo]; only for working trees), and Remotes (via [readRemotes]). All
// derived fields are cleared to zero before population so that Resolve is the single source of truth per
// the "Resolve resolves all metadata, no exceptions" rule.
//
// A path that does not exist, is not a directory, or is not a git repository is not an error: the receiver
// returns with zero-valued metadata and nil error. Callers inspect [Resource.Bare] and the presence of HEAD
// to distinguish "resolved to empty" from "never resolved."
//
// Returns:
//   - error: currently always nil; reserved for future error channels (e.g., surfacing `git` binary
//     unavailability as an explicit condition instead of silently treating it as "no repo").
func (r *Resource) Resolve() error {

	root := r.ExecutionContext().Root
	r.SourcePath = root.NewPath(r.SourcePath.Abs())

	r.Ref, r.HEAD, r.Bare, r.Dirty, r.Remotes = "", "", false, false, nil

	abs := r.SourcePath.Abs()

	repo, bare := isGitRepo(abs)
	if !repo {
		return nil
	}

	r.Bare = bare
	r.HEAD = readHEADSha(abs)
	r.Ref = readBranchName(abs)
	r.Remotes = readRemotes(abs)

	if !bare {
		r.Dirty = isDirtyRepo(abs)
	}

	return nil
}

// String returns a debug-oriented single-line representation of the resource suitable for log lines and IDE
// debug windows.
//
// Returns:
//   - string: "git.Resource{uri=<URI>, ref=<ref>, head=<head>, bare=<bool>, dirty=<bool>, remotes=<count>}".
func (r *Resource) String() string {
	return fmt.Sprintf("git.Resource{uri=%s, ref=%s, head=%s, bare=%t, dirty=%t, remotes=%d}",
		r.URI(), r.Ref, r.HEAD, r.Bare, r.Dirty, len(r.Remotes))
}

// UnmarshalJSON populates the receiver from its JSON wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before
// invoking this method. Identity is reconstructed via [NewResource] from the URI; Ref and HEAD are assigned
// from the decoded snapshot. Operational state (Remotes, Bare, Dirty) stays at zero values until
// [Resource.Resolve] reads the on-disk clone.
//
// Parameters:
//   - data: JSON-encoded wire form.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the JSON does not decode, or resource construction
//     fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("git.Resource: UnmarshalJSON requires ExecutionContext on receiver")
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

	built, err := NewResource(r.ExecutionContext(), aux.URI)
	if err != nil {
		return err
	}

	built.Ref = ref
	built.HEAD = head

	*r = *built
	return nil
}

// UnmarshalStarlark populates the receiver from a [starlark.String] containing a local path or file URI.
//
// Scalar form: only identity (URI) round-trips. Ref, HEAD, and Remotes remain at zero values; richer
// round-trip uses [Resource.UnmarshalJSON] or [Resource.UnmarshalYAML].
//
// Parameters:
//   - sv: a starlark value expected to be a [starlark.String].
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the value is not a [starlark.String], or resource
//     construction fails.
func (r *Resource) UnmarshalStarlark(sv starlark.Value) error {

	if r.ExecutionContext() == nil {
		return errors.New("git.Resource: UnmarshalStarlark requires ExecutionContext on receiver")
	}

	s, ok := sv.(starlark.String)
	if !ok {
		return fmt.Errorf("git.Resource: expected starlark.String, got %s", sv.Type())
	}

	built, err := NewResource(r.ExecutionContext(), string(s))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a local path or file URI.
//
// Scalar form: only identity (URI) round-trips. Ref, HEAD, and Remotes remain at zero values; richer
// round-trip uses [Resource.UnmarshalJSON] or [Resource.UnmarshalYAML].
//
// Parameters:
//   - text: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing or resource construction fails.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("git.Resource: UnmarshalText requires ExecutionContext on receiver")
	}

	built, err := NewResource(r.ExecutionContext(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before
// invoking this method. Identity is reconstructed via [NewResource] from the URI; Ref and HEAD are assigned
// from the decoded snapshot. Operational state (Remotes, Bare, Dirty) stays at zero values until
// [Resource.Resolve] reads the on-disk clone.
//
// Parameters:
//   - unmarshal: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the YAML does not decode, or resource construction
//     fails.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.ExecutionContext() == nil {
		return errors.New("git.Resource: UnmarshalYAML requires ExecutionContext on receiver")
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

	built, err := NewResource(r.ExecutionContext(), aux.URI)
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

// Remote carries the fetch and push URLs for a named git remote.
//
// PushURL is empty when the push direction uses FetchURL (git's default) — the distinction matters for workflows
// that split read and write endpoints (e.g., HTTPS fetch / SSH push, mirror fetch / authoritative push).
type Remote struct {
	FetchURL string
	PushURL  string
}
