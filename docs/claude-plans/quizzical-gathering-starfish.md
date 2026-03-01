# PR Preparation Plan

## Branch Info
- **Branch**: `chore/update-claude-rules-and-ssh-config`
- **Commit**: `f59aad62 chore | CLAUDE.md rules and ssh config update`
- **Target**: `main`

## Changes
1. **Home/Configs/all/.claude/CLAUDE.md** (+11 lines)
   - Adds "Plan Documents" section with workflow for creating plans

2. **Home/Configs/thenobles/.ssh/config.d/home-local** (+2/-2 lines)
   - Updates `dockerhost-us-wa-1` and `dockerhost-us-wa-2` to use `id_ed25519` instead of `id_rsa`

## Steps
1. Push branch to origin with `-u` flag
2. Create PR targeting `main` with summary of changes

## PR Details
**Title**: `chore | CLAUDE.md rules and ssh config update`

**Body**:
```
## Summary
- Add "Plan Documents" section to CLAUDE.md with workflow for creating plan documents
- Update SSH config for dockerhosts to use ed25519 keys instead of RSA

## Test plan
- [x] Changes are configuration only - no runtime testing required
```
