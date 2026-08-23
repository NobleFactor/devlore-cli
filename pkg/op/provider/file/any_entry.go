// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// AnyEntry is the taxonomy variant that asserts existence and nothing else — the unasserted claim
// (docs/plans/any-entry-claims.md, ruled 2026-08-23).
//
// The kinded variants declare what must be at a path; AnyEntry declines to, which is what a
// kind-indifferent operation needs: a move moves whatever is there, and the author should not have to
// name a kind the operation does not care about. Its assertion is exactly "some taxonomy entry exists at
// this rel" — a dangling symbolic link satisfies it (the link is there), a FIFO, socket, or device does
// not (the taxonomy has no variant for them).
//
// An unasserted claim is in effect a promise to observe: the entry resolves to the variant the disk
// shows inside the [op.Pending] → [op.Active] transition, where the model first consults the disk. A
// claim that fails to activate stays an AnyEntry — nothing was observed, so there is nothing to resolve
// to, and the [op.Gone] entry honestly records an unmet unasserted claim.
//
// Identity is the embedded [entry] (URI + SourcePath), and the serialized intent row names this type —
// intent says *something must be here*; the trace says what was found.
type AnyEntry struct {
	entry
}

// sealedEntry marks AnyEntry as a member of the closed [Entry] set (step 23, slice 4).
func (*AnyEntry) sealedEntry() {}

// region EXPORTED METHODS

// region State management

// Digest returns the honest content hash of whatever the disk holds, by delegating to the observed
// kind's implementation.
//
// An unasserted claim still owes content identity: without this, an AnyEntry entry would record no
// digest and silently lose drift detection for as long as it remains unresolved. The observed kind's
// own contract applies verbatim — a directory's digest rule, a link's target hash, a regular file's
// streamed sha256.
//
// Returns:
//   - `op.Digest`: the observed kind's digest.
//   - `error`: an lstat failure, an unsupported entry kind, or the kind's own digest error.
func (r *AnyEntry) Digest() (op.Digest, error) {

	observed, err := r.observed()
	if err != nil {
		return op.Digest{}, err
	}

	return observed.Digest()
}

// Etag returns the cheap change-detection token of whatever the disk holds, by delegating to the
// observed kind's implementation — the screen half of the same contract [AnyEntry.Digest] serves.
//
// Returns:
//   - `string`: the observed kind's etag.
//   - `error`: an lstat failure, an unsupported entry kind, or the kind's own etag error.
func (r *AnyEntry) Etag() (string, error) {

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
func (r *AnyEntry) Exists() bool {

	root := r.RuntimeEnvironment().Root()

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return false
	}

	return EntryKindEntry.admits(info.Mode())
}

// endregion

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
//   - `Entry`: the observed-kind candidate at this path.
//   - `error`: an lstat failure or an entry kind the taxonomy has no variant for.
func (r *AnyEntry) observed() (Entry, error) {

	root := r.RuntimeEnvironment().Root()
	abs := r.SourcePath.Abs()

	info, err := root.Lstat(root.NewPath(abs))
	if err != nil {
		return nil, fmt.Errorf("file.AnyEntry: %s: %w", r.SourcePath.Rel(), err)
	}

	return candidateOfMode(r.RuntimeEnvironment(), abs, info.Mode())
}

// endregion

// region HELPER FUNCTIONS

// DiscoverAnyEntry registers a [file.AnyEntry] via [op.ResourceCatalog.Discover] without claiming
// production — the constructor plan-time claiming and rehydration both key on.
//
// AnyEntry has no producing counterpart: a product is minted as its observed kind (the producer is at
// execution time with the disk in hand), so nothing ever produces an unasserted entry. Nil-catalog
// tolerance returns the unlinked candidate.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*AnyEntry`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the catalog's strict assertions fail, or the URI's existing
//     entry is another kind.
func DiscoverAnyEntry(runtimeEnvironment *op.RuntimeEnvironment, value any) (*AnyEntry, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*AnyEntry]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, "", false, &AnyEntry{entry: *base})
}

// endregion
