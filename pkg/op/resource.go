// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"net/url"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Resource is the interface for all resource types.
//
// Every provider-specific resource (e.g., file.Resource) must embed [ResourceBase] to satisfy it. The unexported
// resourceBase method seals the interface to package op. Only types embedding [ResourceBase] can implement [Resource].
type Resource interface {
	URI() string
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

// URI returns the canonical URI of this resource.
func (b ResourceBase) URI() string {
	return b.uri
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
)

// MarshalStarvalue implements [starvalue.Marshaler]. It serializes the
// private identity fields (uri, id, originID) so they survive the
// Go → Starlark → Go round-trip used by [FillSlot].
func (b ResourceBase) MarshalStarvalue() (starlark.Value, error) {
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

// TombstoneBase holds the resource that was affected by a compensable action. Provider-specific tombstone types must
// embed it by value.
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

// ResourceURI builds a canonicalized URI from a scheme and an identifier.
// For file:// URIs, id is resolved via filepath.Abs + filepath.Clean.
// All URIs use the authority form with an empty host: scheme:///id.
func ResourceURI(scheme, id string) string {
	if scheme == SchemeFile {
		abs, err := filepath.Abs(id)
		if err == nil {
			id = abs
		}
		id = filepath.Clean(id)
	}
	if len(id) == 0 || id[0] != '/' {
		id = "/" + id
	}
	return (&url.URL{Scheme: scheme, Path: id}).String()
}
