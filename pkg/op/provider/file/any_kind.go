// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// AnyKind is the sealed interface over an entry of any kind: the taxonomy variant that asserts existence and
// nothing else — the unasserted claim (docs/plans/any-entry-claims.md, ruled 2026-08-23).
//
// The kinded variants declare what must be at a path; AnyKind declines to, which is what a kind-indifferent
// operation needs: a move moves whatever is there, and the author should not have to name a kind the operation
// does not care about. Its assertion is exactly "some taxonomy entry exists at this rel" — a dangling symbolic
// link satisfies it (the link is there), a FIFO, socket, or device does not (the taxonomy has no variant for them).
//
// An unasserted claim is in effect a promise to observe: the entry resolves to the variant the disk shows inside
// the [op.Pending] → [op.Active] transition, where the model first consults the disk. A claim that fails to
// activate stays an AnyKind — nothing was observed, so there is nothing to resolve to, and the [op.Gone] entry
// honestly records an unmet unasserted claim. An authored string bound to a slot typed [Resource] claims as this
// variant.
//
// Identity is the embedded [resource] (URI + SourcePath), and the serialized intent row names this type — intent
// says *something must be here*; the trace says what was found.
//
// `kind() AnyKind` is the discriminator (ruling 6). Go compares full method signatures, result types included, so
// a type satisfying AnyKind cannot satisfy another variant, and a type may declare only one method named `kind`,
// so no concrete type can be two kinds at once. The variants' exported method sets are otherwise identical,
// which is why a bare interface would not do: `r.(Directory)` would succeed for a regular file.
type AnyKind interface {
	Resource

	// kind marks the closed set: only this package can declare it, and its result type names the kind.
	kind() AnyKind
}

// Interface guard: the unexported struct is the only AnyKind implementation.
var _ AnyKind = (*anyKind)(nil)

// anyKind is the concrete resource behind [AnyKind] — what serializes, and the only thing that implements it.
//
// Unexported so that `&file.anyKind{...}` cannot be written anywhere else. The exported constructors are the
// public contract; the struct behind them need not be.
type anyKind struct {
	resource
}

// kind is [AnyKind]'s discriminator: its result type is what tells this variant from the other three.
//
// Returns:
//   - `AnyKind`: the receiver.
func (r *anyKind) kind() AnyKind { return r }

// sealedResource marks AnyKind as a member of the closed [Resource] set (step 23, slice 4).
func (*anyKind) sealedResource() {}

// region EXPORTED METHODS

// region State management

// Digest returns the honest content hash of whatever the disk holds, by delegating to the observed
// kind's implementation.
//
// An unasserted claim still owes content identity: without this, an AnyKind entry would record no
// digest and silently lose drift detection for as long as it remains unresolved. The observed kind's
// own contract applies verbatim — a directory's digest rule, a link's target hash, a regular file's
// streamed sha256.
//
// Returns:
//   - `op.Digest`: the observed kind's digest.
//   - `error`: an lstat failure, an unsupported entry kind, or the kind's own digest error.
func (r *anyKind) Digest() (op.Digest, error) {

	observed, err := r.observed()
	if err != nil {
		return op.Digest{}, err
	}

	return observed.Digest()
}

// Etag returns the cheap change-detection token of whatever the disk holds, by delegating to the
// observed kind's implementation — the screen half of the same contract [AnyKind.Digest] serves.
//
// Returns:
//   - `string`: the observed kind's etag.
//   - `error`: an lstat failure, an unsupported entry kind, or the kind's own etag error.
func (r *anyKind) Etag() (string, error) {

	observed, err := r.observed()
	if err != nil {
		return "", err
	}

	return observed.Etag()
}

// Exists reports whether ANY taxonomy entry exists at this resource's path — lstat, no follow, no kind
// assertion.
//
// The permissive predicate, and deliberately not the kinded variants' lstat-plus-kind-test nor a
// following [os.Stat]: a dangling symbolic link must count as present (the link is the entry), and an
// entry the taxonomy has no variant for — a FIFO, socket, or device — must not, because no claim could
// legitimately resolve to it.
//
// Returns:
//   - `bool`: true when the path holds a regular file, a directory, or a symbolic link.
func (r *anyKind) Exists() bool {

	mode, present := r.observedMode()

	return present && ResourceKindAny.admits(mode)
}

// MismatchesKind reports whether the path holds an entry no taxonomy variant covers — a FIFO, a socket,
// a device ([op.KindMismatcher]).
//
// Even the unasserted claim has a bound: it asserts *some taxonomy entry*, so a kind nothing could
// resolve to is a surprise rather than an absence, and a surprise is never tolerable.
//
// Returns:
//   - `bool`: true when an entry is there and no variant admits it.
func (r *anyKind) MismatchesKind() bool {

	mode, present := r.observedMode()

	return present && !ResourceKindAny.admits(mode)
}

// endregion

// ResolveKind returns the taxonomy variant the disk currently holds — the promise to observe, come due
// (4-resource-management.md; [op.KindResolver]).
//
// An unasserted claim asserts existence and defers the kind; pre-flight's Pending → Active transition is
// where the model first looks, so it is where the deferral ends. The returned variant is freshly built
// and uninterned: the catalog stamps identity across the swap, because identity is the catalog's
// business, not this resource's.
//
// Returns:
//   - `op.Resource`: the observed-kind variant at this path.
//   - `error`: an lstat failure, or an entry kind the taxonomy has no variant for.
func (r *anyKind) ResolveKind() (op.Resource, error) {

	return r.observed()
}

// endregion

// region UNEXPORTED METHODS

// observed mints the unlinked taxonomy candidate for whatever the disk currently holds at this
// resource's path — the delegation target for the content-identity tiers.
//
// The candidate is deliberately uninterned: it is a behavior handle, not an identity. This entry's
// identity is already established, and minting a second cataloged entry for the same rel is exactly the
// cross-kind duplication the catalog forbids.
//
// Returns:
//   - `Resource`: the observed-kind candidate at this path.
//   - `error`: an lstat failure or an entry kind the taxonomy has no variant for.
func (r *anyKind) observed() (Resource, error) {

	root := r.RuntimeEnvironment().Root()
	abs := r.SourcePath.Abs()

	info, err := root.Lstat(root.NewPath(abs))
	if err != nil {
		return nil, fmt.Errorf("file.AnyKind: %s: %w", r.SourcePath.Rel(), err)
	}

	return candidateOfMode(r.RuntimeEnvironment(), abs, info.Mode())
}

// endregion

// region HELPER FUNCTIONS

// DiscoverAnyKind registers a [file.AnyKind] via [op.ResourceCatalog.Discover] without claiming
// production — the constructor plan-time claiming and rehydration both key on.
//
// AnyKind has no producing counterpart: a product is minted as its observed kind (the producer is at
// execution time with the disk in hand), so nothing ever produces an unasserted entry. Nil-catalog
// tolerance returns the unlinked candidate.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// **Returns the taxonomy interface, not `AnyKind`** — the one constructor that does, and deliberately. An
// unasserted claim is satisfied by whatever entry already stands for its identity, and a kinded entry
// asserts more, so it keeps the ledger slot (the collision rule: one rel, one identity, the stricter
// assertion wins). A constructor that promised `AnyKind` would have to fail in exactly the case the rule
// says should succeed.
//
// Returns:
//   - `Resource`: the canonical catalog entry — a fresh [AnyKind] when the identity is unclaimed, the
//     existing kinded entry when it is not, or the unlinked candidate when no catalog is present.
//   - `error`: if `value` is not a string, or the catalog's strict assertions fail.
func DiscoverAnyKind(runtimeEnvironment *op.RuntimeEnvironment, value any) (Resource, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[AnyKind]())
	if err != nil {
		return nil, err
	}

	candidate := &anyKind{resource: *base}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.Discover(
		candidate.URI(), func() (op.Resource, error) { return candidate, nil })
	if err != nil {
		return nil, err
	}

	entry, isEntry := got.(Resource)
	if !isEntry {
		return nil, fmt.Errorf("file: catalog entry for %q is %T, want a file resource", candidate.URI(), got)
	}

	return entry, nil
}

// endregion
