// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Clone clones a git repository.
type Clone struct{ Impl *Provider }

func (o *Clone) Name() string { return "git.clone" }

func (o *Clone) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	url, _ := slots["url"].(string)
	if url == "" {
		return nil, nil, fmt.Errorf("git-clone: no url specified")
	}
	path, _ := slots["path"].(string)
	if path == "" {
		return nil, nil, fmt.Errorf("git-clone: no path specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] git-clone %v \u2192 %v\n", url, path)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Clone(url, path, ctx.Logger)
}

func (o *Clone) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Checkout checks out a git ref.
type Checkout struct{ Impl *Provider }

func (o *Checkout) Name() string { return "git.checkout" }

func (o *Checkout) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	repo, _ := slots["path"].(string)
	if repo == "" {
		return nil, nil, fmt.Errorf("git-checkout: no path specified")
	}
	ref, _ := slots["ref"].(string)
	if ref == "" {
		return nil, nil, fmt.Errorf("git-checkout: no ref specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] git-checkout %v %v\n", repo, ref)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Checkout(repo, ref, ctx.Logger)
}

func (o *Checkout) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Pull pulls latest changes in a git repository.
type Pull struct{ Impl *Provider }

func (o *Pull) Name() string { return "git.pull" }

func (o *Pull) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	repo, _ := slots["path"].(string)
	if repo == "" {
		return nil, nil, fmt.Errorf("git-pull: no path specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] git-pull %v\n", repo)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Pull(repo, ctx.Logger)
}

func (o *Pull) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Register registers all git actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Clone{Impl: p})
	reg.Register(&Checkout{Impl: p})
	reg.Register(&Pull{Impl: p})
}
