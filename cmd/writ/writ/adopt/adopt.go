// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package adopt plans the `writ adopt` batch graph.
//
// The cobra layer (`cmd/writ/writ/adopt_cmd.go`) hands over the inputs — the locations of the files to adopt — and
// the scopes this platform defines; [Collect] enumerates the inputs into per-scope [Item] batches, each item in the
// scope whose root is the deepest that holds it (#926); [BuildGraph] turns one batch into one execution graph: a
// deduplicated mkdir pre-stage, then one move-and-link chain per item (#931). The existing-destination guard runs
// before the graph, in [RunBatches] (#939).
package adopt
