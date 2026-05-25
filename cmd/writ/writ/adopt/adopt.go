// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package adopt holds the per-file scaffolding for the `writ adopt` workflow.
//
// Phase 6.B (this commit) creates the subpackage as a stub; Phase 6.C fills in the iteration scaffolding
// (`AdoptFiles` / `AdoptItem`) that today's monolithic `adoptFile` helper in `cmd/writ/writ/adopt_cmd.go`
// performs inline. The split mirrors the established `cmd/writ/writ/migrate_cmd.go` + `cmd/writ/writ/migrate/`
// precedent: the cobra glue stays at the parent layer; the per-file iteration logic and the
// graph-construction / execution helpers move into this subpackage for isolated unit testability.
package adopt
