# Plan: Workflow Fix and Doc Update

## Tasks

### 1. Fix sync-knowledge workflow
**File:** `.github/workflows/sync-knowledge.yaml`

Remove the `paths:` filters from both `push` and `pull_request` triggers (lines 12-16 and 22-26). The workflow should run on every push/PR to protected branches. The star tool will determine if there are actual knowledge changes to commit.

### 2. Fix llm-integration.md documentation
**File:** `docs/architecture/llm-integration.md`

Update the code example at lines 62-66 to match the actual config types:
- `model.Config` → `config.ModelConfig`
- `Model:` → `Name:`

### 3. Clean up stashes
- Drop stash 0 (config consolidation changes now covered by tasks above)
- Drop stash 1 (workflow changes are stale/superseded)
