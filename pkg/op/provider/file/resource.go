// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource is this provider's resource type — the sealed interface the taxonomy's variants implement.
//
// Every provider names its resource type `Resource`; file's is an interface rather than a struct because file is the
// one provider with a kind axis. Modeled after the standard library's fs.DirEntry precedent (phase-8 step 23,
// ruling 4): contexts that legitimately traffic in mixed or observed kinds — enumeration returns, per-entry walker
// callbacks, observation minting, kind-indifferent mutation — accept or return a Resource rather than a concrete
// variant. Contexts whose semantics fix the kind use the variant ([*Regular], [*Directory], [*SymbolicLink])
// directly; [*AnyKind] is the variant that asserts existence without asserting kind.
type Resource interface {
	op.Resource

	// Path returns the canonicalized absolute path handle on the disk.
	Path() fsroot.Path

	// sealedResource marks the closed set of Resource implementations (step 23, slice 4): each taxonomy variant — and
	// only a variant — declares it. The unexported base deliberately does not, so a hand-built base value cannot
	// enter any taxonomy signature, and packages outside this one cannot add implementations.
	sealedResource()
}

// Interface guards: the four taxonomy variants are the only Resource implementations — the seal excludes the
// unexported base by construction (it lacks the marker).
var (
	_ Resource = (*AnyKind)(nil)
	_ Resource = (*Regular)(nil)
	_ Resource = (*Directory)(nil)
	_ Resource = (*SymbolicLink)(nil)
)
