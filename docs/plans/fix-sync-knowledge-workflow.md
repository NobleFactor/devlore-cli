# Plan: Fix sync-knowledge Workflow

---
title: Fix sync-knowledge Workflow
issue: https://github.com/NobleFactor/devlore-cli/issues/86
status: blocked
created: 2026-02-10
updated: 2026-02-10
blocked_by: https://github.com/NobleFactor/devlore-cli/issues/84
---

## Summary

Fix the sync-knowledge GitHub Actions workflow that fails after PR #85 introduced the `star/extensions/` directory structure. Three issues were discovered; two are fixed, one requires implementation work.

## Quick Start for Future Sessions

**To pick up this work:**

1. Read this plan
2. Implement Phase 2 (choose Option A, B, or C based on user preference)
3. If Option B chosen, also implement Phase 3

**Current blocker:** The Starlark script `build-knowledge.star` calls `go.parse_devlore_api()` but no binary has that receiver wired up.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Binary path | :white_check_mark: Fixed | Changed to `bin/star` |
| Extension discovery | :white_check_mark: Fixed | Run from devlore-cli directory |
| Receiver `go.parse_devlore_api` | :x: Blocking | Not wired to any binary |

## Error Progression

**Error 1** - Binary path conflict (FIXED):
```
./star: Is a directory
```

**Error 2** - Extension discovery (FIXED):
```
Error: unknown command "devlore-registry" for "star"
```

**Error 3** - Missing receiver (CURRENT BLOCKER):
```
Error: go has no .parse_devlore_api attribute
  build-knowledge.star:79:13: in build_onboarding_knowledge
```

## Implementation Options

### Option A: Disable workflow temporarily

**Effort**: 5 minutes
**Trade-off**: Knowledge sync stops until proper fix

Edit `.github/workflows/sync-knowledge.yaml`:

```yaml
jobs:
  sync-knowledge:
    runs-on: ubuntu-latest
    # Temporarily disabled - requires devlore receiver (#84)
    if: false
    steps:
      # ... rest unchanged
```

Then commit and push.

---

### Option B: Wire up receiver to lore/writ binary

**Effort**: 30-60 minutes
**Trade-off**: Proper fix, requires code changes

The `lore` and `writ` binaries already exist in devlore-cli. Wire the receiver into one of them.

#### Step 1: Find the Starlark runtime initialization

Look in `cmd/lore/main.go` or `cmd/writ/main.go` for where Starlark globals are set up. Also check `internal/cli/` for command execution.

```bash
grep -r "starlark.StringDict\|NewThread\|ExecFile" internal/ cmd/
```

#### Step 2: Create receiver wrapper

The receiver code already exists. Create a wrapper in `internal/starlark/receivers.go`:

```go
// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/starlark/devlore"
)

// GoReceiver provides Go source parsing operations.
type GoReceiver struct{}

func (r *GoReceiver) String() string        { return "go" }
func (r *GoReceiver) Type() string          { return "receiver" }
func (r *GoReceiver) Freeze()               {}
func (r *GoReceiver) Truth() starlark.Bool  { return true }
func (r *GoReceiver) Hash() (uint32, error) { return 0, nil }

func (r *GoReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "parse_devlore_api":
		return starlark.NewBuiltin("go.parse_devlore_api", devlore.GoParseDevloreAPI), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("go has no .%s attribute", name))
	}
}

func (r *GoReceiver) AttrNames() []string {
	return []string{"parse_devlore_api"}
}
```

#### Step 3: Register receiver in globals

Where Starlark scripts are executed, add the receiver to globals:

```go
globals := starlark.StringDict{
	"go": &starlark.GoReceiver{},
	// ... other existing globals
}
```

#### Step 4: Update workflow to use lore/writ

Edit `.github/workflows/sync-knowledge.yaml`:

```yaml
- name: Build devlore tools
  working-directory: devlore-cli
  run: go build -o bin/lore ./cmd/lore

- name: Build knowledge base
  working-directory: devlore-cli
  run: |
    ./bin/lore devlore-registry build knowledge \
      --domain all \
      --source_path ${{ github.workspace }}/devlore-cli \
      --registry_path ${{ github.workspace }}/devlore-registry
```

---

### Option C: Stub the receiver in Starlark

**Effort**: 10 minutes
**Trade-off**: Partial functionality, knowledge sync runs but skips API parsing

Edit `star/extensions/com.noblefactor.devlore.registry.BuildKnowledge/commands/build-knowledge.star`:

Find line 79 where `go.parse_devlore_api` is called and wrap it:

```starlark
def build_onboarding_knowledge(source_path, registry_path):
    # ... existing code up to the parse call ...

    # Check if receiver is available (may not be when running with generic star)
    if hasattr(go, "parse_devlore_api"):
        api_info = go.parse_devlore_api(source_path)
    else:
        warn("Skipping API parsing - go.parse_devlore_api not available")
        warn("Run with devlore-cli binary for full functionality")
        api_info = {}

    # ... rest of function ...
```

Do the same for any other `go.*` calls in the registry extensions.

---

## Recommended Approach

1. **Immediate**: Apply Option A or C to unblock PR #85
2. **Follow-up**: Implement Option B as a separate PR

## Files Reference

| File | Purpose |
| --- | --- |
| `internal/starlark/devlore/api.go` | `GoParseDevloreAPI` function - the receiver implementation |
| `star/extensions/com.noblefactor.devlore.registry.BuildKnowledge/commands/build-knowledge.star:79` | Where receiver is called |
| `.github/workflows/sync-knowledge.yaml` | The failing workflow |
| `cmd/lore/main.go` | lore binary entry point |
| `cmd/writ/main.go` | writ binary entry point |

## GoParseDevloreAPI Signature

For reference, the existing function in `internal/starlark/devlore/api.go`:

```go
func GoParseDevloreAPI(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
```

This parses Go source files in devlore-cli to extract Starlark API binding information. It returns a Starlark dict with the parsed data.

## Acceptance Criteria

- [ ] sync-knowledge workflow passes on PR #85
- [ ] If Option B: `go.parse_devlore_api()` callable from Starlark
- [ ] If Option C: Graceful degradation when receiver unavailable

## Commit Messages

**For Option A:**
```
fix(ci): disable sync-knowledge until receiver wired (#84)
```

**For Option B:**
```
feat(starlark): wire up devlore receiver to lore binary

Closes #84
```

**For Option C:**
```
fix(starlark): gracefully handle missing go receiver

The build-knowledge extension now checks for receiver availability
before calling go.parse_devlore_api, allowing it to run with the
generic star binary from noblefactor-ops.
```

## Related Documents

- [Issue #86](https://github.com/NobleFactor/devlore-cli/issues/86) - This workflow issue
- [Issue #84](https://github.com/NobleFactor/devlore-cli/issues/84) - Wire up devlore receiver (BLOCKING)
- [PR #85](https://github.com/NobleFactor/devlore-cli/pull/85) - Add star extensions
- [PR #52 noblefactor-ops](https://github.com/NobleFactor/noblefactor-ops/pull/52) - Removed devlore from ops