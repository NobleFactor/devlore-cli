// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"reflect"

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
	_ Resource = (*anyKind)(nil)
	_ Resource = (*regular)(nil)
	_ Resource = (*directory)(nil)
	_ Resource = (*symbolicLink)(nil)
)

// init wires what a sealed variant cannot state from its generated announcement.
//
// Two things per variant, both needed because the announcement lives in a sibling package and can name only
// exported identifiers: the struct reflection runs against, in the NON-pointer form the announcement passed
// while the variants were structs (receiver construction promotes it to a pointer itself), and the struct an
// authored string claims as. A `Resource`-typed slot claims as [AnyKind] — existence without kind, resolved
// to what the disk holds at activation (4-resource-management.md §5.7 rule 6) — so kind-indifferent methods
// can take the base interface and still be authored with a plain path.
func init() {

	op.RegisterResourceImplementation(reflect.TypeFor[AnyKind](), reflect.TypeFor[anyKind]())
	op.RegisterResourceImplementation(reflect.TypeFor[Regular](), reflect.TypeFor[regular]())
	op.RegisterResourceImplementation(reflect.TypeFor[Directory](), reflect.TypeFor[directory]())
	op.RegisterResourceImplementation(reflect.TypeFor[SymbolicLink](), reflect.TypeFor[symbolicLink]())

	op.RegisterResourceMint(reflect.TypeFor[Resource](), reflect.TypeFor[*anyKind]())
	op.RegisterResourceMint(reflect.TypeFor[AnyKind](), reflect.TypeFor[*anyKind]())
	op.RegisterResourceMint(reflect.TypeFor[Regular](), reflect.TypeFor[*regular]())
	op.RegisterResourceMint(reflect.TypeFor[Directory](), reflect.TypeFor[*directory]())
	op.RegisterResourceMint(reflect.TypeFor[SymbolicLink](), reflect.TypeFor[*symbolicLink]())
}
