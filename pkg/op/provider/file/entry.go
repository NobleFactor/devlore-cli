// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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
}

// Interface guards: the three taxonomy variants — and, until the slice-4 seal retires its constructibility, the
// catch-all base — are the Entry implementations.
var (
	_ Entry = (*Regular)(nil)
	_ Entry = (*Directory)(nil)
	_ Entry = (*SymbolicLink)(nil)
	_ Entry = (*Resource)(nil)
)
