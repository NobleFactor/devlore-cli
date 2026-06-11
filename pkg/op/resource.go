// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// tagURIPrefix is the fixed prefix of every canonical [Resource] URI.
//
// It is an RFC 4151 tag URI of the form "tag:<authority>,<date>:" where the authority and date are locked constants.
// The authority identifies the devlore project, and the date identifies the entitlement epoch (not the mint time).
const tagURIPrefix = "tag:devlore.noblefactor.com,2026-01-01:"

var (
	// ErrUnimplemented is returned by [op.ResourceBase.Digest] as a default. Concrete Resource types that need a
	// working Digest (every type save sentinels) must override [Resource.Digest] — content hashing is type-specific
	// (full file sha256, HEAD commit composition, last-observed body hash, projected from the URI for CAS, etc.).
	ErrUnimplemented = errors.New("op: unimplemented")

	// stringType is the cached [reflect.Type] of the Go string type, consulted by [op.ResourceBase.CanConvert] and
	// [ResourceBase.Convert] to decide whether the URI projection applies to a given conversion target.
	stringType = reflect.TypeFor[string]()
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

	ID() string
	URI() string
	Addressing() AddressingMode
	Digest() (Digest, error)
	Etag() (string, error)
	ProducerID() string

	resourceBase() *ResourceBase
}

// ResourceBase holds the identity fields common to all resources.
//
// ReceiverType-specific resource receiverTypes must embed it by value. The uri, specific, and typeID fields are set at
// construction via [NewResourceBase]: uri is the minted canonical tag URI, specific is the scheme-specific identity
// payload, typeID is the canonical Go type id of the concrete Resource type. The id and producerID fields are stamped
// by the [ResourceCatalog] when the resource is cataloged; they are not a concern of the resource itself.
type ResourceBase struct {
	ProviderBase
	id         string
	producerID string
	specific   string
	typeID     string
	uri        string
}

// NewResourceBase constructs a ResourceBase whose identity is the canonical tag URI.
//
// tag:devlore.noblefactor.com,2026-01-01:<specific>#<typeID>, where <typeID> is goType's canonical Go type id
// (PkgPath() + "." + Name()). Pointer types are normalized to their element.
//
// An empty <specific> is valid and produces the deferred ("known-at-execution") form — the shape constructed by
// [op.Defer] when a resource's identity is not known until the producing node has executed.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via ProviderBase.
//   - `specific`: the scheme-specific identity payload. Must not contain '#', reserved as the fragment delimiter.
//   - `goType`: the concrete Go type whose identity is placed in the fragment.
//
// Returns:
//   - `ResourceBase`: the constructed base with uri, specific, and typeID all populated.
//   - `error`: non-nil when specific contains '#' or goType has empty PkgPath and Name.
func NewResourceBase(runtimeEnvironment *RuntimeEnvironment, specific string, goType reflect.Type) (ResourceBase, error) {

	if strings.Contains(specific, "#") {
		return ResourceBase{}, fmt.Errorf("op.NewResourceBase: specific %q contains '#', which is reserved as the fragment delimiter", specific)
	}

	typeID := typeIDOf(goType)

	if typeID == "." {
		return ResourceBase{}, fmt.Errorf("op.NewResourceBase: goType has empty PkgPath and Name")
	}

	uri := specific

	if !strings.HasPrefix(uri, tagURIPrefix) {
		uri = tagURIPrefix + specific
	}

	return ResourceBase{
		ProviderBase: NewProviderBase(runtimeEnvironment),
		uri:          uri + "#" + typeID,
		specific:     specific,
		typeID:       typeID,
	}, nil
}

// region EXPORTED METHODS

// region State management

// ID returns the catalog-stamped identity of this resource.
func (b *ResourceBase) ID() string {
	return b.id
}

// ProducerID returns the catalog-stamped producer node ID.
func (b *ResourceBase) ProducerID() string {
	return b.producerID
}

// ReachabilityURI returns the scheme-specific identity payload — the <specific> portion of the canonical tag URI.
//
// Empty for resources constructed via [Defer] or otherwise in the deferred ("known-at-execution") form.
func (b *ResourceBase) ReachabilityURI() string {
	return b.specific
}

// ResourceType returns the canonical Go type id of the concrete Resource type — the fragment portion of the canonical
// tag URI.
func (b *ResourceBase) ResourceType() string {
	return b.typeID
}

// URI returns the cached canonical tag URI of this resource.
func (b *ResourceBase) URI() string {
	return b.uri
}

// endregion

// region Behaviors

// Fallible actions

// ConvertTo projects this resource into the given target Go type.
//
// The baseline projection is URI → string, matching [ResourceBase.CanConvert]. Concrete Resource types that recognize
// additional targets override [ResourceBase.Convert] and delegate to this method for the string case.
//
// Parameters:
//   - `target`: the destination Go type the caller wants to project the resource into.
//
// Returns:
//   - `any`: the resource's URI (as a Go string) when `target` is string.
//   - `error`: non-nil if `target` is not a conversion this base recognizes.
func (b *ResourceBase) ConvertTo(target reflect.Type) (any, error) {

	if target == stringType {
		return b.uri, nil
	}

	return nil, fmt.Errorf("resource: cannot convert %s to %s", b.uri, target)
}

// Digest returns [ErrUnimplemented].
//
// Concrete Resource types must override — content hashing is type-specific (full file sha256, HEAD commit
// composition, last-observed body hash, projected from the URI for CAS, etc.).
//
// Returns:
//   - Digest: the zero value.
//   - error: always [ErrUnimplemented].
func (b *ResourceBase) Digest() (Digest, error) {
	return Digest{}, ErrUnimplemented
}

// Etag returns the URI as the inexpensive change-detection token.
//
// Suggestive of change but not authoritative; the catalog computes [Resource.Digest] only when Etag mismatches what's
// stored. This default is correct for resources with [AddressingContent] by definition. The same URI implies the
// contents are immutable, so the URI itself doubles as the etag at no I/O cost. [AddressingLocation] subtypes override
// with their own stamp (size + mtime + inode for files; HTTP ETag header for appnet; etc.).
//
// Returns:
//   - `string`: the URI.
//   - `error`: nil.
func (b *ResourceBase) Etag() (string, error) {
	return b.uri, nil
}

// MarshalJSON marshals the resource to its JSON wire form, which is the URI as a JSON-encoded string.
//
// The URI is the resource's identity and the only field required for a round trip through JSON: catalog rehydration
// reconstructs the resource via [NewResource] from the stored URI. Concrete Resource types that need to persist
// additional fields (cached metadata, domain-specific state) override [ResourceBase.MarshalJSON] with their own
// serialization.
//
// Returns:
//   - `[]byte`: the JSON-encoded URI string.
//   - `error`: any error from [json.Marshal]; none under normal conditions.
func (b *ResourceBase) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.uri)
}

// MarshalText marshals the resource to its text wire form, which is the URI as raw UTF-8 bytes.
//
// The text form is consumed by stdlib encoders ([encoding/json] for map keys, [encoding/xml] for attributes), YAML
// scalar emission via [yaml.v3], CLI flag ingestion via [flag.TextVar], and most env/config parsers. Round trip through
// [UnmarshalText] (implemented per concrete Resource type) reconstructs an equivalent resource.
//
// Returns:
//   - `[]byte`: the URI as UTF-8 bytes.
//   - `error`: nil under normal conditions; included to satisfy the [encoding.TextMarshaler] interface.
func (b *ResourceBase) MarshalText() ([]byte, error) {
	return []byte(b.uri), nil
}

// MarshalYAML marshals the resource for YAML encoding as a bare string scalar — the URI.
//
// Returning a plain string (rather than a struct) yields a clean YAML scalar in serialized form, avoiding the
// nested-object shape that reflection-based YAML marshaling would produce. Concrete Resource types that need to persist
// additional fields override [ResourceBase.MarshalYAML] with their own representation.
//
// Returns:
//   - `any`: the URI string.
//   - `error`: nil under normal conditions; included to satisfy the yaml.Marshaler interface.
func (b *ResourceBase) MarshalYAML() (any, error) {
	return b.uri, nil
}

// Resolve populates provider-specific metadata via I/O (e.g., os.Stat for files).
//
// The default implementation is a no-op — providers that need resolution (file, git) override it. Callers that need
// metadata call Resolve then check the result. An unresolved resource reports Exists() == false. Implementations access
// the confined fsroot via RuntimeEnvironment().Root.

// Actions

// Addressing returns [AddressingUnknown] as a sentinel default.
//
// Every concrete Resource type must override to return one of [AddressingLocation] or [AddressingContent]. The
// boot-discipline test in pkg/op/addressing_test.go (added in 13.0(k) sub-step k.12) walks every announced Resource
// type and asserts none returns [AddressingUnknown].
//
// Returns:
//   - AddressingMode: [AddressingUnknown].
func (b *ResourceBase) Addressing() AddressingMode {
	return AddressingUnknown
}

// CanConvertTo reports whether this resource can project itself into the given target Go type.
//
// The baseline projection is URI → string: any ResourceBase knows how to produce its URI as a Go string. Concrete
// Resource types extend this by overriding [ResourceBase.CanConvert] to accept additional targets (e.g., a
// [file.Resource] that projects to an fsroot.Path) and delegating to this method for the string case.
//
// Parameters:
//   - `target`: the destination Go type the caller wants to project the resource into.
//
// Returns:
//   - `bool`: true if `target` is the Go string type; false otherwise.
func (b *ResourceBase) CanConvertTo(target reflect.Type) bool {
	return target == stringType
}

// Equal reports whether b and other identify the same resource.
//
// Equality is URI-based and loose with respect to the concrete Go type: any two values implementing [Resource] whose
// URIs match are equal. A URI collision across concrete types (e.g., a file URI embedded in an appnet.Resource) is
// treated as a caller-side construction error, not a case Equal needs to disambiguate — the URI is the sole identity.
//
// Contract (mirroring the [java.lang.Object.equals] properties):
//   - Reflexive: b.Equal(b) returns true.
//   - Symmetric: b.Equal(x) returns true iff x.Equal(b) returns true.
//   - Transitive: if b.Equal(x) and x.Equal(y), then b.Equal(y).
//   - Consistent: repeated calls return the same result while URIs are stable.
//   - Nil-safe: b.Equal(nil) returns false.
//
// Parameters:
//   - `other`: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - `bool`: true if other is a [Resource] with the same URI as b.
func (b *ResourceBase) Equal(other any) bool {

	if other == nil {
		return false
	}

	o, ok := other.(Resource)
	if !ok {
		return false
	}

	return b.uri == o.URI()
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

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// resourceBase returns a pointer to the embedded ResourceBase, allowing the catalog to stamp id and producerID.
//
// This method seals the Resource interface.
func (b *ResourceBase) resourceBase() *ResourceBase {
	return b
}

// endregion

// endregion

// region HELPER FUNCTIONS

// Defer constructs a placeholder instance of *R with a deferred tag URI — empty <specific>, typeID set to *R's
// canonical Go type id.
//
// Use at plan time when a Resource's identity is not known until the producing node has executed. The returned value is
// a freshly allocated *R whose embedded [ResourceBase] has been seeded by [NewResourceBase] against the deferred
// identity.
//
// Type parameters:
//   - R: the struct type of the Resource (e.g., yaml.Resource).
//   - PR: the pointer type *R that satisfies [Resource]. The "*R; Resource" constraint is statically enforced at
//     the call site; invalid combinations fail to compile.
//
// Call sites must spell both parameters:
//
//	r := op.Defer[yaml.Resource, *yaml.Resource](runtimeEnvironment)
func Defer[R any, PR interface {
	*R
	Resource
}](runtimeEnvironment *RuntimeEnvironment) PR {

	v := PR(new(R))

	base, err := NewResourceBase(runtimeEnvironment, "", reflect.TypeFor[PR]())
	assert.NoError("op.Defer", err)

	*v.resourceBase() = base
	return v
}

// ExtractTagSpecific parses a canonical tag URI and returns its scheme-specific payload and fragment.
//
// Returns an error when s lacks the tag URI prefix, is missing the '#' delimiter, or has an empty fragment. An empty
// specific is valid and denotes the deferred ("known-at-execution") form.
//
// Parameters:
//   - `value`: the URI to parse.
//
// Returns:
//   - `specific`: the scheme-specific payload (this may be empty and indicates that it's unknown at the moment).
//   - `typeID`: the fragment — the canonical Go type id of the Resource type.
//   - `err`: non-nil on any syntactic defect.
func ExtractTagSpecific(value string) (specific, typeID string, err error) {

	if !strings.HasPrefix(value, tagURIPrefix) {
		return "", "", fmt.Errorf("op.ExtractTagSpecific: %q lacks prefix %q", value, tagURIPrefix)
	}

	rest := value[len(tagURIPrefix):]

	var found bool
	specific, typeID, found = strings.Cut(rest, "#")

	if !found {
		return "", "", fmt.Errorf("op.ExtractTagSpecific: %q has no '#' fragment delimiter", value)
	}

	if typeID == "" {
		return "", "", fmt.Errorf("op.ExtractTagSpecific: %q has empty fragment", value)
	}

	return specific, typeID, nil
}

// typeIDOf returns the canonical Go type id for goType: PkgPath() + "." + Name().
//
// Pointer types are normalized to their element.
func typeIDOf(goType reflect.Type) string {
	if goType.Kind() == reflect.Pointer {
		goType = goType.Elem()
	}
	return goType.PkgPath() + "." + goType.Name()
}

// endregion
