// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"net/url"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Resource is the interface for all resource types.
//
// Every provider-specific resource (e.g., file.Resource) must embed [ResourceBase] to satisfy it. The unexported
// resourceBase method seals the interface to package op. Only types embedding [ResourceBase] can implement [Resource].
//
// Concrete types must implement [Scheme], [Host], and [Path] to provide URI components. [URI] should be implemented
// as a one-liner delegating to [ResourceBase.NewURI]:
//
//	func (r *Resource) URI() string { return r.NewURI(r) }
//
// [ResourceBase] provides default implementations of all four methods by parsing the stored uri field. Concrete types
// shadow these with efficient, component-based implementations.
type Resource interface {
	URI() string
	Scheme() string
	Host() string
	Path() string
	Resolve() error
	resourceBase() *ResourceBase
}

// ResourceBase holds the identity fields common to all resources. Provider-
// specific resource types must embed it by value.
//
// The uri field is set at construction via [NewResourceBase]. The id and
// originID fields are stamped by the [ResourceCatalog] when the resource
// is cataloged; they are not a concern of the resource itself.
type ResourceBase struct {
	uri      string
	id       string
	originID string
}

// NewResourceBase creates a ResourceBase with the given URI.
func NewResourceBase(uri string) ResourceBase {
	return ResourceBase{uri: uri}
}

// URI returns the canonical URI of this resource. Concrete types should
// shadow this with: func (r *Resource) URI() string { return r.NewURI(r) }
func (b *ResourceBase) URI() string {
	return b.uri
}

// Scheme returns the URI scheme by parsing the stored uri. Concrete types
// should shadow this with a constant (e.g., "file").
func (b *ResourceBase) Scheme() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

// Host returns the URI host by parsing the stored uri. Concrete types
// should shadow this with their authority component (often empty).
func (b *ResourceBase) Host() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Host
}

// Path returns the URI path by parsing the stored uri. Concrete types
// should shadow this with their provider-specific identifier.
func (b *ResourceBase) Path() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Path
}

// NewURI builds a canonical URI from a concrete Resource's component methods.
// Concrete types call this to implement URI():
//
//	func (r *Resource) URI() string { return r.NewURI(r) }
func (b *ResourceBase) NewURI(r Resource) string {
	return (&url.URL{Scheme: r.Scheme(), Host: r.Host(), Path: r.Path()}).String()
}

// Resolve populates provider-specific metadata via I/O (e.g., os.Stat for
// files). The default implementation is a no-op — providers that need
// resolution (file, git) override it. Callers that need metadata call
// Resolve() then check the result. An unresolved resource reports
// Exists() == false.
func (b *ResourceBase) Resolve() error { return nil }

// resourceBase returns a pointer to the embedded ResourceBase, allowing the
// catalog to stamp id and originID. This method seals the Resource interface.
func (b *ResourceBase) resourceBase() *ResourceBase {
	return b
}

// URI scheme constants.
const (
	SchemeFile    = "file"
	SchemeGit     = "git"
	SchemePackage = "pkg"
	SchemeService = "svc"
	SchemeMem     = "mem"
	SchemeNet     = "net"
)

// MarshalStarvalue implements [starvalue.Marshaler]. It serializes the
// private identity fields (uri, id, originID) so they survive the
// Go → Starlark → Go round-trip used by [FillSlot].
func (b *ResourceBase) MarshalStarvalue() (starlark.Value, error) {
	return starlarkstruct.FromStringDict(starlark.String("resource_base"), starlark.StringDict{
		"uri":       starlark.String(b.uri),
		"id":        starlark.String(b.id),
		"origin_id": starlark.String(b.originID),
	}), nil
}

// Tombstone is the interface for all compensation state types.
//
// Every provider-specific tombstone (e.g., file.Tombstone) must embed [TombstoneBase] to satisfy it. The unexported
// tombstoneBase method seals the interface to types that embed [TombstoneBase].
type Tombstone interface {
	Resource() Resource
	tombstoneBase()
}

// TombstoneBase holds the resource that was affected by a compensable action.
// Provider-specific tombstone types must embed it by value.
//
// The embedded Resource reflects post-operation state: its identity fields
// (e.g., SourcePath for file resources) point to where the data physically
// IS after the operation, not where it was before. Provider-specific fields
// on the tombstone (e.g., file.Tombstone.OriginalPath) record where the
// data came from — the restoration target.
type TombstoneBase struct {
	resource Resource
}

// NewTombstoneBase creates a TombstoneBase anchored to the given resource.
func NewTombstoneBase(resource Resource) TombstoneBase {
	return TombstoneBase{resource: resource}
}

// Resource returns the resource affected by the compensable action.
func (b TombstoneBase) Resource() Resource {
	return b.resource
}

func (b TombstoneBase) tombstoneBase() {}

// NoResult signals that a provider method produces no output. Used by
// CompensableAction methods like Remove and RemoveAll that can be undone
// but produce no result for downstream nodes. classifyActionReturn maps
// NoResult to nil Result; classifyReturn maps it to starlark.None.
type NoResult struct{}
