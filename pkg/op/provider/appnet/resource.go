// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a network resource identified by a URL.
//
// The URI IS the canonical full URL — scheme, host, path, and query. The scheme is preserved because it's
// reachability-critical (http vs. https vs. ftp are distinct endpoints). Canonicalization normalizes host casing,
// strips default ports, normalizes percent-encoding, strips trailing slashes, collapses repeated slashes, and sorts
// query parameters — all semantics-preserving transforms over the given URL.
//
// Two URLs that differ only in the transport scheme (e.g., "http://x" vs. "https://x") produce distinct resources.
// This is a deliberate consequence of "identity ensures reachability." Consumers that want transport-independent
// addressing should factor that into their own logic; appnet.Resource does not.
//
// SourceURL is a non-persisted *url.URL view of the URI, populated at construction (and reparsed on unmarshal) for
// callers that want structured URL access. It always equals url.Parse(URI).
type Resource struct {
	op.ResourceBase

	// SourceURL is a parsed view of the URI. Derived from URI at construction and on unmarshal; not
	// persisted (the URI on ResourceBase is authoritative).
	SourceURL *url.URL `json:"-" yaml:"-"`
}

// NewResource constructs an appnet.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped
// with `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles,
// scanner-style discovery, the framework's slot-coercion adapter).
//
// Today no appnet provider method actually claims production — Download returns []byte, not *Resource;
// future fetchers (e.g., 13.0(k.10)'s Download → *stream.Resource) would produce a stream.Resource, not
// an appnet.Resource. NewResource exists for symmetry with the m.4 two-constructor pattern and as a
// stable surface for any future appnet producer.
//
// The URL is canonicalized (lowercase host, strip default port, normalize percent-encoding, strip trailing
// slash, collapse double slashes, sort query parameters) while preserving the transport scheme. The
// canonicalized URL becomes the Resource's URI; SourceURL is populated by reparsing it.
//
// Nil-Catalog tolerance mirrors [DiscoverResource]: when `runtimeEnvironment.Catalog` is nil (test
// fixtures, library callers without a runtime), the candidate is returned unlinked.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `unit`: the producing [op.ExecutableUnit] whose ID becomes the catalog entry's producerID, or nil
//     for non-graph dispatch.
//   - `value`: a string URL with a transport scheme.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, does not parse as a URL, or has no scheme.
func NewResource(runtimeEnvironment *op.RuntimeEnvironment, unit op.ExecutableUnit, value any) (*Resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.GetOrCreate(
		unit,
		candidate.URI(),
		func() (op.Resource, error) {
			return candidate, nil
		})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("appnet.NewResource: catalog entry for %q is %T, want *appnet.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource registers an appnet.Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string and
// the slot expects a *appnet.Resource), and by callers that hold a reference handle without claiming to
// have produced the underlying URL endpoint (UnmarshalJSON/Text/YAML rehydration is the canonical example).
//
// Discover does not stamp a producer, so unlike [NewResource] it takes only `runtimeEnvironment` — no
// unit reference is needed.
//
// Nil-Catalog tolerance mirrors the receipt-rehydration paths.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string URL with a transport scheme.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, does not parse as a URL, or has no scheme.
func DiscoverResource(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

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

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf(
			"appnet.DiscoverResource: catalog entry for %q is %T, want *appnet.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate constructs a canonical *Resource from `value` without touching the catalog.
//
// Validates that `value` is a string URL with a transport scheme and canonicalizes the URL. Shared by
// [NewResource] and [DiscoverResource].
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment threaded into the produced [op.ResourceBase].
//   - `value`: a string URL; any other type is an error.
//
// Returns:
//   - *Resource: the canonicalized candidate, not yet interned in the catalog.
//   - `error`: if `value` is not a string, does not parse as a URL, or has no transport scheme.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	raw, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("appnet.Resource: expected string URL, got %T", value)
	}

	canonical, err := canonicalURL(raw)
	if err != nil {
		return nil, err
	}

	if canonical.Scheme == "" {
		return nil, fmt.Errorf("appnet.Resource: URL missing transport scheme: %q", raw)
	}

	base, err := op.NewResourceBase(runtimeEnvironment, canonical.String(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourceURL:    canonical,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that appnet.Resource is location-keyed: the canonical URL is the identity.
//
// The bytes served at that URL are not part of this Resource's identity — that concern belongs to a
// separate stream-shaped Resource (planned: stream.Resource in 13.0(k) sub-step k.10), which Download
// will eventually return instead of bare bytes.
//
// Returns:
//   - op.AddressingMode: always [op.AddressingLocation].
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Digest returns sha256 of the canonical URL.
//
// The bytes served at the URL are not part of identity here (see [Resource.Addressing]); content
// addressing of fetched bytes is the future stream.Resource's job. Hashing the URL keeps the digest
// consistent in algorithm with the rest of the system (the catalog's [op.ParseDigest] only accepts
// sha256) and gives appnet.Resource a stable, content-addressable token derived from its identity.
//
// Returns:
//   - op.Digest: sha256 algorithm with 32 raw bytes.
//   - `error`: nil under normal conditions.
func (r *Resource) Digest() (op.Digest, error) {
	h := sha256.Sum256([]byte(r.URI()))
	return op.Digest{Algorithm: "sha256", Bytes: h[:]}, nil
}

// Equal reports whether r and other identify the same appnet resource.
//
// Strict equality: other must be a *appnet.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - `bool`: true if `other` is a *appnet.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns the canonical URL itself.
//
// For a URL-keyed Resource, the URL IS the change-detection token — two appnet.Resources with the same
// URL are the same Resource (same etag); two with different URLs are different Resources (different URI,
// different catalog entry, no shadowing involved). The catalog's Etag fast-path therefore always matches
// for an unchanged appnet.Resource.
//
// Returns:
//   - `string`: the canonical URL (identical to [op.ResourceBase.URI]).
//   - `error`: nil under normal conditions.
func (r *Resource) Etag() (string, error) {
	return r.URI(), nil
}

// String returns a debug-oriented single-line representation of the resource.
//
// Returns:
//   - `string`: `appnet.Resource{uri=<URI>}`.
func (r *Resource) String() string {
	return fmt.Sprintf("appnet.Resource{uri=%s}", r.URI())
}

// CanConvertFrom reports whether `source` can be projected into a [*Resource] via [Resource.ConvertFrom].
//
// Opts the appnet Resource into the framework's [op.TargetConverter] contract — accepted source shape is
// `string` (interpreted as a URL). The framework consults this probe both at plan-time via
// [op.typesAreInterconvertible] (the bubble-up parameter-consistency check) and at dispatch-time via
// [op.Convert] step 7 (env-less fallback). The canonical dispatch-time path remains the registered
// constructor at [op.Convert] step 6, which receives the full [op.RuntimeEnvironment] and canonicalizes the
// URL via [buildCandidate].
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
// only the SourceURL parsed from `value`; the canonical URI on the embedded [op.ResourceBase] is NOT
// populated here. Provider methods consuming the projected Resource are responsible for re-canonicalization
// via their own [NewResource]/[DiscoverResource] path when full identity is required.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*Resource].
//   - `error`: non-nil when `value` is not a `string` or does not parse as a URL.
func (*Resource) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("appnet.Resource.ConvertFrom: source must be string, got %T", value)
	}

	u, err := url.Parse(str)
	if err != nil {
		return nil, fmt.Errorf("appnet.Resource.ConvertFrom: parse URL %q: %w", str, err)
	}

	return &Resource{SourceURL: u}, nil
}

// endregion

// region Behaviors

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URL string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method. The URL alone is sufficient — identity IS reachability.
//
// Parameters:
//   - `data`: JSON-encoded wire form (a bare URL string).
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, JSON decode failure, or rehydration failure.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("appnet.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URL.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URL.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("appnet.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form (a bare URL scalar).
//
// Parameters:
//   - `unmarshal`: yaml decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("appnet.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// canonicalURL parses value, normalizes it in place, and returns the result.
//
// Normalization (RFC 3986 semantics-preserving):
//   - Lowercase hostname.
//   - Strip default port for the transport (80 for http, 443 for https, 21 for ftp).
//   - Normalize percent-encoding: uppercase hex digits, decode unreserved characters.
//   - Strip trailing /.
//   - Collapse repeated // in path (preserving leading /).
//   - Sort query parameters by key (url.Values.Encode sorts alphabetically).
//
// The transport scheme is preserved — it's reachability-critical and part of the resource's identity.
//
// Parameters:
//   - `value`: the raw URL string to parse and canonicalize.
//
// Returns:
//   - *url.URL: the canonicalized URL.
//   - `error`: non-nil when `value` does not parse as a URL.
func canonicalURL(value string) (*url.URL, error) {

	sourceURL, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("appnet.Resource: invalid URL %q: %w", value, err)
	}

	host := strings.ToLower(sourceURL.Hostname())
	port := sourceURL.Port()

	if port != "" && !isDefaultPort(sourceURL.Scheme, port) {
		host += ":" + port
	}

	p := normalizePercentEncoding(sourceURL.EscapedPath())
	p = collapseSlashes(p)
	p = strings.TrimRight(p, "/")

	if p == "" {
		p = "/"
	}

	q := sourceURL.Query().Encode()

	sourceURL.Host = host
	sourceURL.RawPath = p
	sourceURL.Path, _ = url.PathUnescape(p)
	sourceURL.RawQuery = q

	return sourceURL, nil
}

// isDefaultPort returns true if port matches the well-known default for scheme.
//
// Parameters:
//   - `scheme`: the URL transport scheme (e.g., "http", "https", "ftp").
//   - `port`: the port string to test.
//
// Returns:
//   - `bool`: true when `port` is the well-known default for `scheme`.
func isDefaultPort(scheme, port string) bool {

	defaults := map[string]string{
		"http":  "80",
		"https": "443",
		"ftp":   "21",
	}
	return defaults[scheme] == port
}

// collapseSlashes replaces runs of multiple / with a single / in the path, preserving the leading /.
//
// Parameters:
//   - `p`: the path to normalize.
//
// Returns:
//   - `string`: the path with repeated slashes collapsed.
func collapseSlashes(p string) string {

	if p == "" {
		return p
	}

	lead := ""
	rest := p
	if strings.HasPrefix(p, "/") {
		lead = "/"
		rest = strings.TrimLeft(p, "/")
	}
	if rest == "" {
		return lead
	}

	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' })
	return lead + strings.Join(parts, "/")
}

// normalizePercentEncoding uppercases hex digits in percent-encoded sequences and decodes unreserved characters.
//
// Unreserved characters are the characters that are allowed in a URI without being percent-encoded. According to
// RFC 3986 §2.3, the unreserved set consists of:
//
// - Uppercase and Lowercase Letters: A–Z and a–z
// - Decimal Digits: 0–9
// - Hyphen: -
// - Period: .
// - Underscore: _
// - Tilde: ~
//
// See: https://datatracker.ietf.org/doc/html/rfc3986#section-2.3
//
// Parameters:
//   - `s`: the URL path or query segment to normalize.
//
// Returns:
//   - `string`: the input with hex digits uppercased and unreserved characters decoded.
func normalizePercentEncoding(s string) string {

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi := upperHex(s[i+1])
			lo := upperHex(s[i+2])
			decoded := unhex(hi)<<4 | unhex(lo)
			if isUnreserved(decoded) {
				b.WriteByte(decoded)
			} else {
				b.WriteByte('%')
				b.WriteByte(hi)
				b.WriteByte(lo)
			}
			i += 2
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// isUnreserved returns true for RFC 3986 §2.3 unreserved characters.
//
// Parameters:
//   - `c`: the byte to classify.
//
// Returns:
//   - `bool`: true when `c` is in the unreserved set (alphanumeric, `-`, `.`, `_`, `~`).
func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~'
}

// upperHex converts a hex digit to uppercase.
//
// Parameters:
//   - `c`: a hex digit byte (`0`–`9`, `a`–`f`, `A`–`F`).
//
// Returns:
//   - `byte`: the input with lowercase `a`–`f` uppercased; other bytes pass through unchanged.
func upperHex(c byte) byte {
	if c >= 'a' && c <= 'f' {
		return c - ('a' - 'A')
	}
	return c
}

// unhex converts a hex digit to its numeric value.
//
// Parameters:
//   - `c`: a hex digit byte (`0`–`9`, `a`–`f`, `A`–`F`).
//
// Returns:
//   - `byte`: the numeric value 0–15; 0 for any non-hex byte.
func unhex(c byte) byte {

	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}
