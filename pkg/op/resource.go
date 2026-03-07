// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"
	"net/url"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Resource is the interface for all resource types.
//
// Every provider-specific resource (e.g., file.Resource) must embed [ResourceBase] to satisfy it. The unexported
// resourceBase method seals the interface to package op. Only types embedding [ResourceBase] can implement [Resource].
//
// URI() returns a cached string computed at construction time. Each concrete type owns its URI construction —
// there is no shared dispatch. If Resolve() changes identity-bearing fields (e.g., path canonicalization),
// the concrete type updates the cached URI via [ResourceBase.SetURI].
type Resource interface {
	URI() string
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

// URI returns the cached canonical URI of this resource.
func (b *ResourceBase) URI() string {
	return b.uri
}

// SetURI updates the cached URI. Concrete types call this after Resolve()
// when identity-bearing fields change (e.g., path canonicalization).
func (b *ResourceBase) SetURI(uri string) {
	b.uri = uri
}

// Scheme returns the URI scheme by parsing the stored uri.
// Convenience helper — NOT an interface method.
func (b *ResourceBase) Scheme() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

// Opaque returns the opaque data component of the URI (non-empty for
// opaque URIs like pkg:, svc:, mem:, appnet:). For hierarchical URIs
// (file://), returns empty. Convenience helper — NOT an interface method.
func (b *ResourceBase) Opaque() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Opaque
}

// Host returns the URI host by parsing the stored uri. Non-empty for
// hierarchical URIs with an authority (e.g., net://host/path). Empty
// for opaque URIs. Convenience helper — NOT an interface method.
func (b *ResourceBase) Host() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Host
}

// Path returns the URI path by parsing the stored uri. Non-empty for
// hierarchical URIs. Empty for opaque URIs. Convenience helper — NOT
// an interface method.
func (b *ResourceBase) Path() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Path
}

// Fragment returns the URI fragment by parsing the stored uri.
// Convenience helper — NOT an interface method.
func (b *ResourceBase) Fragment() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Fragment
}

// Resolve populates provider-specific metadata via I/O (e.g., os.Stat for
// files). The default implementation is a no-op — providers that need
// resolution (file, git) override it. Callers that need metadata call
// Resolve() then check the result. An unresolved resource reports
// Exists() == false.
func (b *ResourceBase) Resolve() error { return nil }

// Format marshals v as compact JSON. Concrete resource types call this from
// their String() method: func (r Resource) String() string { return r.Format(r) }
func (b ResourceBase) Format(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

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
	SchemeAppNet  = "appnet"
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
