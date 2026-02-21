# Phase 1: Fix the realtime_receiver Template

**Status**: COMPLETE — PR #151

## Summary

The `realtime_receiver` template (builtin in noblefactor-ops) generates receivers
that call `host.Host` methods. This phase modifies it to generate receivers that
call Provider methods instead.

## Changes

### noblefactor-ops

- `internal/starlark/receiver_go_gen.go` — new `realtimeProviderBody` template helper
- `internal/starlark/receiver_go_gen_test.go` — test for new helper

### devlore-cli

- `star/extensions/com.noblefactor.devlore.Actions/templates/realtime_receiver.go.template` — local template replacing builtin
- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` — update LOCAL_TEMPLATES

## Design Decisions

- Provider structs get optional dependency fields (e.g., `pkg.Provider{PM: pm}`)
- Generated receiver constructors set these fields
- Actions continue using slots for dependency injection
