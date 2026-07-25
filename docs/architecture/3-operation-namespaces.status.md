# Status: Operation Namespaces

**Architecture document:** [3-operation-namespaces.md](3-operation-namespaces.md)

**State:** rewritten 2026-07-22 (phase-8 step 51, slice 5). The pre-`op` how-to — `internal/execution` paths,
hand-written `actions_gen.go` `Do`/`Undo` wrappers, `op.Announce` descriptors with `Register`/`NewPlanned`
callbacks, `SetSlotImmediate`, the `plan.package.*` examples, and the stale namespace tables (which listed
`flow.elevate`/`flow.fatal`, `ui.success`, `shell.power_shell`, and pre-taxonomy file actions) — is replaced by the
landed authoring workflow: hand-written provider (ProviderBase, access directive, activation-first, receipt-paired
compensation) → `make generate` (star-LKG codegen: `AnnounceProvider` metadata + typed `ActionName` constants +
`New-OpInventory` blank-import rosters) → tests (unit + fixture + the announce-in-test pattern) → catalog row +
3.5.x design doc. The provider inventory is no longer duplicated here — [3.5](3.5-provider-catalog.md) owns it.
The "object of the action" contract section is kept, restated on current signatures.

## Completion

| Component | Status |
|-----------|--------|
| Announce-with-metadata registration (`AnnounceProvider`) + inventory generation | Landed |
| Typed action-name constants (step 32) | Landed |
| Activation-first floor (step 27) + receipt-paired compensation (steps 40/42) | Landed |
| The three-tier Starlark surface + root promotion ([3.5.3](3.5.3-plan-provider.md)) | Landed |
| Document rewrite onto the landed workflow | Complete 2026-07-22 (step 51 slice 5) |

## Document Discrepancies

None known — the 2026-07-22 rewrite grounds every claim in the current tree (Makefile `generate`/`inventory`
targets, the gen-file shapes, the `pkg/op/server` announce-in-test pattern).

## Outstanding Work

None for this document. Remaining step-51 slices are tracked in
[step 51](../plans/extract-starlark-from-op/phase-8/steps/51-documentation-debt.md).
