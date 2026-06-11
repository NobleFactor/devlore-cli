// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

import (
	"context"
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Run dispatches the adopt graph via the supplied [*op.GraphExecutor] and translates any aggregated D5 envelope
// error into the per-stage prefix style the pre-13.0(n) `adoptFile` helper returned, so the CLI UX is byte-equivalent
// across the migration.
//
// Phase 6.C Q4 lock: single-error mapping. The D5 envelope frequently aggregates several preflight failures (e.g.,
// missing dest_dir + missing source_path) but each adopt invocation processes one file; the user sees one fsroot cause
// per failed file. [mapAdoptError] unwraps the first joined error from the envelope (when [errors.Join] is the cause)
// and applies the file-stage prefix matching the original inline messages:
//
//   - Mkdir failure  → "creating directory %s: %w"   (dest_dir resolved from the envelope's context, falling back to
//     the bare error when the stage can't be inferred)
//   - Move failure   → "moving file: %w"
//   - Link failure   → "creating symlink (file remains at %s): %w"
//
// The framework's own error messages — variable resolution (`parameter %q: %s value of type %T not assignable to
// declared type %s`), preflight (`parameter %q (%s) is required but no source supplied a value`) — pass through with
// their existing wording; adopt only adds its file-stage prefix when the executor's per-node error surfaces.
//
// Parameters:
//   - `ctx`: the parent context; passed verbatim to [op.GraphExecutor.Run].
//   - `executor`: the executor bound to the adopt graph and the writ spec; constructed by the caller via
//     [op.NewGraphExecutor].
//
// Returns:
//   - `error`: non-nil on preflight or dispatch failure, mapped to the legacy CLI prefix style.
func Run(ctx context.Context, executor *op.GraphExecutor) error {

	_, err := executor.Run(ctx, nil)
	if err != nil {
		return mapAdoptError(err)
	}

	return nil
}

// mapAdoptError translates an [op.GraphExecutor.Run] error into the per-stage prefix style the pre-13.0(n) inline
// `adoptFile` helper returned. Walks [errors.Join] aggregations to pick the first member when present; matches the
// inferred file-stage by substring against the canonical action labels (file.mkdir, file.move, file.link) and applies
// the corresponding prefix. Falls through with the raw error when no stage can be inferred — preserves D5 envelope
// errors verbatim for the user when they don't match a known prefix pattern.
//
// Parameters:
//   - `err`: the error returned by [op.GraphExecutor.Run].
//
// Returns:
//   - `error`: the prefix-mapped error; nil when the input is nil.
func mapAdoptError(err error) error {

	if err == nil {
		return nil
	}

	first := firstJoinedError(err)
	msg := first.Error()

	switch {
	case strings.Contains(msg, "file.mkdir"):
		return fmt.Errorf("creating destination directory: %w", first)
	case strings.Contains(msg, "file.move"):
		return fmt.Errorf("moving file: %w", first)
	case strings.Contains(msg, "file.link"):
		return fmt.Errorf("creating symlink: %w", first)
	default:
		return err
	}
}

// firstJoinedError returns the first member of an [errors.Join] aggregation, or the input error unchanged when it
// is not a Join. Used by [mapAdoptError] so the per-stage prefix wraps the single most-relevant fsroot cause when the
// D5 preflight envelope happens to aggregate multiple unrelated failures for the same file.
//
// Parameters:
//   - `err`: the candidate aggregated error.
//
// Returns:
//   - `error`: the first joined member when err is an [errors.Join] aggregation; err unchanged otherwise.
func firstJoinedError(err error) error {

	type unwrapper interface {
		Unwrap() []error
	}

	if u, ok := err.(unwrapper); ok {
		members := u.Unwrap()
		if len(members) > 0 {
			return members[0]
		}
	}

	return err
}
