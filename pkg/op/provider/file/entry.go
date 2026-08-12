// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Entry is the mixed-kind currency for "any file resource" — the interface the taxonomy's variants implement.
//
// Modeled after the standard library's fs.DirEntry precedent (phase-8 step 23, ruling 4): contexts that legitimately
// traffic in mixed or observed kinds — enumeration returns, per-entry walker callbacks, observation minting — accept
// or return an Entry rather than a concrete variant. Contexts whose semantics fix the kind use the concrete variant
// ([*Regular], [*Directory], [*SymbolicLink]) directly, and a plain string path is the currency for
// create/update/delete parameters (ruling 2 — the resource is the product of a mutation, never its input).
type Entry interface {
	op.Resource

	// Path returns the canonicalized absolute path handle on the disk.
	Path() fsroot.Path

	// sealedEntry marks the closed set of Entry implementations (step 23, slice 4): each taxonomy variant — and
	// only a variant — declares it. The catch-all base deliberately does not, so a hand-built base value cannot
	// enter any taxonomy signature, and packages outside this one cannot add implementations.
	sealedEntry()
}

// Interface guards: the three taxonomy variants are the only Entry implementations — the seal excludes the
// catch-all base by construction (it lacks the marker).
var (
	_ Entry = (*Regular)(nil)
	_ Entry = (*Directory)(nil)
	_ Entry = (*SymbolicLink)(nil)
)
