// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Resource is the interface for all resource receiverTypes.
//
// Every provider-specific resource (e.g., file.Resource) must embed [ResourceBase] to satisfy it. The unexported
// resourceBase method seals the interface to package op. Only receiverTypes embedding [ResourceBase] can implement
// [Resource].
//
// URI() returns an immutable string computed at construction time. Each concrete type's NewResource constructor
// formulates the URI from the value descriptor and execution context. The URI is the resource's identity — it does
// not change after construction. [Resolve] enriches metadata (stat, version) but does not alter identity.
type Resource interface {
	Provider
	URI() string
	Resolve() error
	resourceBase() *ResourceBase
}

// ResourceBase holds the identity fields common to all resources.
//
// ReceiverType-specific resource receiverTypes must embed it by value. The uri field is set at construction via
// [NewResourceBase]. The id and originID fields are stamped by the [ResourceCatalog] when the resource is cataloged;
// they are not a concern of the resource itself.
type ResourceBase struct {
	ProviderBase
	uri      string
	id       string
	originID string
}

// NewResourceBase creates a ResourceBase with the given URI.
func NewResourceBase(ctx *ExecutionContext, uri string) ResourceBase {
	return ResourceBase{
		ProviderBase: NewProviderBase(ctx),
		uri:          uri,
	}
}

// URI returns the cached canonical URI of this resource.
func (b *ResourceBase) URI() string {
	return b.uri
}

// ID returns the catalog-stamped identity of this resource.
func (b *ResourceBase) ID() string {
	return b.id
}

// OriginID returns the catalog-stamped origin node ID.
func (b *ResourceBase) OriginID() string {
	return b.originID
}

// Scheme returns the URI scheme by parsing the stored uri.
//
// Convenience helper--NOT an interface method.
func (b *ResourceBase) Scheme() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

// Opaque returns the opaque data component of the URI (non-empty for opaque URIs like appnet:, mem:, pkg:, svc:).
//
// For hierarchical URIs (file://), returns empty. Convenience helper--NOT an interface method.
func (b *ResourceBase) Opaque() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Opaque
}

// Host returns the URI host by parsing the stored uri.
//
// Non-empty for hierarchical URIs with an authority (e.g., file:///some/path). Empty for opaque URIs. Convenience
// helper — NOT an interface method.
func (b *ResourceBase) Host() string {
	u, err := url.Parse(b.uri)
	if err != nil {
		return ""
	}
	return u.Host
}

// Path returns the URI path by parsing the stored uri.
//
// Non-empty for hierarchical URIs. Empty for opaque URIs. Convenience helper — NOT an interface method.
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

// Format marshals value as compact JSON.
//
// Concrete resource receiverTypes call this from their String() method: func (r Resource) String() string { return
// r.Format(r) }
func (b *ResourceBase) Format(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

// Resolve populates provider-specific metadata via I/O (e.g., os.Stat for files).
//
// The default implementation is a no-op — providers that need resolution (file, git) override it. Callers that need
// metadata call Resolve then check the result. An unresolved resource reports Exists() == false. Implementations access
// the confined root via ExecutionContext().Root.
func (b *ResourceBase) Resolve() error { return nil }

// resourceBase returns a pointer to the embedded ResourceBase, allowing the catalog to stamp id and originID.
//
// This method seals the Resource interface.
func (b *ResourceBase) resourceBase() *ResourceBase {
	return b
}

// KnownAtExecution is the sentinel an output spec returns when the output identity cannot be determined at plan
// time but will be available once the producing node has executed.
//
// The name is temporal, not uncertain: the value is a legitimate resource identity that exists once the producing
// node has run, just not before. Phrasing and semantics borrowed from Terraform's `(known after apply)`.
//
// When the planner sees KnownAtExecution from an output spec, it skips plan-time shadowing for that output. The
// executor shadows the real return value after the forward method returns. Implicit edges via URI matching don't
// work for these outputs at plan time, but explicit promise passing still does.
//
// Typical use:
//
//	func (p *Provider) InstallPlanned(name string, _ string, _ bool) (*Resource, error) {
//	    return op.KnownAtExecution, nil
//	}
//
// See [docs/architecture/4-resource-management.md] §6.8 "Output Specs", "Monadic outputs (unknown at plan time)".
var KnownAtExecution Resource = &knownAtExecution{
	ResourceBase: ResourceBase{uri: "op:known-at-execution"},
}

// knownAtExecution is the unexported type backing the KnownAtExecution sentinel. Callers compare against the
// exported KnownAtExecution variable; the type is not meant to be instantiated directly.
type knownAtExecution struct {
	ResourceBase
}

// IsKnownAtExecution reports whether the given resource is the KnownAtExecution sentinel.
//
// Returns:
//   - bool: true if r is the sentinel, false otherwise (including when r is nil).
func IsKnownAtExecution(r Resource) bool {
	return r == KnownAtExecution
}

// Tombstone is the interface for all compensation state receiverTypes.
//
// Every provider-specific tombstone (e.g., file.Tombstone) must embed [TombstoneBase] to satisfy it. The unexported
// tombstoneBase method seals the interface to receiverTypes that embed [TombstoneBase].
type Tombstone interface {
	Resource() Resource
	tombstoneBase()
}

// TombstoneBase holds the resource that was affected by a compensable do.
//
// ReceiverType-specific tombstone receiverTypes must embed it by value.
//
// The embedded Resource preserves its true identity — its fields are never modified by the recovery system.
// ReceiverType-specific fields on the tombstone (e.g., file.Tombstone.RecoveryID) record where data was temporarily
// moved during the operation — the recovery location, not the identity.
type TombstoneBase struct {
	resource Resource
}

// NewTombstoneBase creates a TombstoneBase anchored to the given resource.
func NewTombstoneBase(resource Resource) TombstoneBase {
	return TombstoneBase{resource: resource}
}

// Resource returns the resource affected by the compensable do.
func (b TombstoneBase) Resource() Resource {
	return b.resource
}

func (b TombstoneBase) tombstoneBase() {}
