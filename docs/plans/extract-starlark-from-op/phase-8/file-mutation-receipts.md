---
title: "Unified file-mutation receipts — one do/undo mechanism for create, update, delete"
status: draft
created: 2026-06-27
---

# Unified file-mutation receipts

## Motivation

`file.write_text`, `file.write_bytes`, and `archive.extract` are three examples of **one operation**: write bytes to a
target path (creating or updating it), displacing any prior content to the recovery site, and producing a `file.Receipt`
that undoes it. They differ only in the **source** of the bytes (inline text, inline bytes, an archive entry) and the
**count** (one vs. many). Deletion (`file.remove`) is the same operation's mirror — its undo is identical (restore the
prior content from recovery).

The undo side is **already unified**: `Provider.compensateWrite` (`file/provider.go:1306`) inverts all three cases —

- **create** (no recovery id): remove the new file, then prune the directories the write created (via the boundary).
- **update** (recovery id from archiving the prior): remove the new file, restore the prior content from recovery.
- **delete** (recovery id, file already gone): remove is a no-op, restore the prior content from recovery.

The forward side is **not** unified, and the routing is broken for any caller that is not a `file` dispatch:

1. The forward is scattered across `prepareWrite`/`write`/`Remove` plus the per-action methods, with no single
   "mutate a file, hand me the self-describing receipt" entry point.
2. **A receipt reaches `compensateWrite` only through its dispatching action.** `RecoveryStack.Unwind` routes by
   `receipt.ActionPath()`, which is stamped at `Commit` from the dispatching unit. A `file.write_text` receipt routes to
   `CompensateWriteText` → `compensateWrite`; an `archive.extract`-built receipt routes to `archive.extract` and never
   gets there. That is the entire reason `archive.extract`'s rollback is a no-op: it pushes the *wrong* receipt — one
   `Unwind` cannot compensate.

The invariant: **if `stack.Unwind()` cannot compensate a receipt, it is the wrong receipt.** A file mutation must
produce a receipt that any stack can undo, regardless of which provider performed it.

## The mechanism

One forward core, one self-describing receipt, one undo.

- **One new forward method, `WriteFile`.** A file write — create/update — streaming `src io.Reader` to disk, archiving
  any prior content to recovery and building the self-describing receipt. It lifts the existing `p.write` →
  `prepareWrite` core, and it is the **only** mutation worth a dedicated method: it factors out non-trivial shared logic
  (streaming + displacement) reused by `WriteText`/`WriteBytes` and `archive.extract`. The other operations need **no**
  new method — `Remove`/`RemoveAll`/`Unlink` already delete (file or empty dir), `Mkdir` already creates a directory,
  and post-slice-2 each returns a self-describing receipt. `DeleteFile`/`MakeDir`/`RemoveDir` would be redundant
  synonyms with no caller, and are dropped (decision 8).
- **One self-describing receipt.** It carries everything the undo needs — `resource`, an explicit `kind`
  (`create|update|delete` file, `create|delete` dir), `recoveryID`, `boundary` — *and* names its own undo, stamped by
  the receipt's **constructor**, because the receipt's *type* knows its undo (`*file.Receipt` is always undone by the
  file provider) regardless of which method or dispatcher created it.
- **One undo.** `compensateWrite`, generalized to `CompensateFileMutation`, inverts any recorded mutation by dispatching
  on the receipt's `kind`. `Unwind` routes to it via the receipt's compensation identity.

### The seam — a receipt names its own undo, two fields with two roles

Compensation routes today off `receipt.actionPath`, which `Commit` stamps from the *dispatching unit* — so a `file`
mutation built by `archive.extract` is stamped `archive.extract` and routes to archive's no-op companion. That one field
has been carrying two unrelated jobs. Split them:

- **`forwardAction`** — the provider method that dispatched (audit; `Trace.Summarize` tally). `Commit` stamps it from
  the dispatching unit, as today.
- **`compensatingAction`** — identifies the compensator (the undo). The receipt's **constructor** stamps it, because the
  type knows its undo: `file.NewReceipt` / `NewReceiptWithBoundary` set it to file's compensator identity, so every
  `*file.Receipt` is born naming its undo — `archive.extract`'s included, since archive builds them *through* those
  constructors. `Commit` stamps `compensatingAction` from the dispatching unit **only when the constructor left it
  empty** — a fallback that is correct for every provider where dispatcher == creator and is kept permanently
  (decision 9).

Resolution stays a *registered* lookup (so it survives a `Trace` reload — a captured closure does not), now keyed on
`compensatingAction`: registration builds a **compensator-name index** (every `Compensate*` method by its dotted name),
and the compensation lookup resolves `compensatingAction` through it, falling back to the existing forward→`.undo` path
for receipts whose `compensatingAction` is a dispatch action (dispatcher-equals-creator providers). The receipt declares its
compensator directly. Because the receipt names its undo, file's per-action `Compensate` companions
(`CompensateWriteText`/`WriteBytes`/`Remove`/`RemoveAll`/`Unlink`/`Mkdir`) collapse into the single
`CompensateFileMutation`, and registration's "a compensable forward needs a `Compensate<Name>` companion" requirement
relaxes (compensation now resolves via `compensatingAction`, not the name convention).

## Proposed API

```go
// Package file

// WriteFile creates or updates the file at target by streaming src to disk (io.Copy — constant memory, and the
// kernel copy_file_range/sendfile fast path when src is an *os.File), displacing any prior content to the recovery
// site. It returns a self-describing *Receipt that names CompensateFileMutation as its undo.
func (p *Provider) WriteFile(target *Resource, src io.Reader, mode os.FileMode) (*Resource, *Receipt, error)

// CompensateFileMutation inverts a recorded mutation, dispatching on the receipt's kind: remove a created file or
// directory, restore prior content from recovery for an update or delete, recreate a removed directory, and prune
// directories the mutation created. It is the single undo named by every file mutation receipt — whether produced by
// WriteFile, by the write/remove/mkdir actions, or by archive.extract. (This is today's compensateWrite, generalized.)
func (p *Provider) CompensateFileMutation(receipt *Receipt) error
```

`WriteFile` is the one new method slice 4 adds: `p.write` taking `src io.Reader` and streaming with `io.Copy`
(replacing the `[]byte` write — true zero-copy via the kernel fast path for file-to-file, constant-memory streaming for a
decompressed archive entry), returning a self-describing receipt. It is the **only** mutation that factors out
non-trivial shared logic (the streaming + displacement core), which is why it earns a method of its own — `WriteText` /
`WriteBytes` wrap it, and `archive.extract` streams entries through it. There is **no** `DeleteFile` / `MakeDir` /
`RemoveDir`: those would be synonyms for the existing `Remove` / `RemoveAll` / `Mkdir`, which already perform the os
operation and (post-slice-2) return self-describing receipts naming `CompensateFileMutation`. A dir entry in
`archive.extract` calls the existing `Mkdir`; deletion stays `file.remove` / `file.remove_all`. See decision 8.

## Usage examples

### file.write_text / file.write_bytes — thin wrappers over the core

```go
func (p *Provider) WriteText(
	activationRecord *op.ActivationRecord, destinationPath string, content string, chmod os.FileMode, chown string,
) (*Resource, *Receipt, error) {

	product, err := NewResource(p.RuntimeEnvironment(), activationRecord.Unit, destinationPath)
	if err != nil {
		return nil, nil, err
	}
	return p.WriteFile(product, strings.NewReader(content), chmod) // self-describing receipt
}
```

`WriteBytes` is identical (`content` is already carried as a string, so the same `strings.NewReader`). Dispatched as a
node, each receipt's compensation action
defaults to its dispatch action (`file.write_text` / `file.write_bytes`), routes through the per-action companion to
`CompensateFileMutation`, and `Unwind` works exactly as it does today.

### archive.extract — a loop over the same core

```go
func (p *Provider) Extract(
	activationRecord *op.ActivationRecord, source *file.Resource, prefixPath string,
) (products []*file.Resource, stack *op.RecoveryStack, err error) {

	runtimeEnvironment := activationRecord.RuntimeEnvironment
	stack = op.NewRecoveryStack()
	fileProvider, err := provider.Instance[file.Provider](runtimeEnvironment)
	if err != nil {
		return nil, nil, err
	}

	reader, err := p.openArchive(source) // a (header, io.Reader) iterator over tar.gz or zip
	if err != nil {
		return nil, nil, err
	}
	defer iox.Close(&err, reader)

	for {
		entry, err := reader.Next() // header + the entry's reader, positioned at its bytes
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return products, stack, fmt.Errorf("archive: read: %w", err)
		}
		target, err := file.NewResource(runtimeEnvironment, activationRecord.Unit,
			filepath.Join(prefixPath, entry.Name))
		if err != nil {
			return products, stack, fmt.Errorf("archive: catalog %q: %w", entry.Name, err)
		}

		// A directory entry makes the dir via the existing Mkdir; a file entry streams straight to disk via WriteFile —
		// the same writes file.mkdir and file.write_bytes perform, no full-content buffer. Either way, one
		// self-describing receipt.
		var receipt *file.Receipt
		if entry.IsDir {
			_, receipt, err = fileProvider.Mkdir(activationRecord, target.SourcePath.Abs(), entry.Mode, "")
		} else {
			_, receipt, err = fileProvider.WriteFile(target, entry.Reader, entry.Mode)
		}
		if err != nil {
			return products, stack, fmt.Errorf("archive: %q: %w", entry.Name, err)
		}

		products = append(products, target)

		if err := stack.Push(receipt, runtimeEnvironment); err != nil {
			return products, stack, fmt.Errorf("archive: push %q: %w", entry.Name, err)
		}
	}

	return products, stack, nil
}

// CompensateExtract collapses to one line: the stack is the complement; the framework unwinds it, and each
// file.Receipt self-routes to file.CompensateFileMutation.
func (p *Provider) CompensateExtract(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}
```

### Undo — the same call everywhere

```go
// In-process saga (mid-extract failure → undo before any retry): the boundary unwinds the partial stack.
err := stack.Unwind() // each entry → file.CompensateFileMutation: remove written, restore displaced, prune dirs

// Or, ad hoc, the receipt from any single write:
_, receipt, _ := fileProvider.WriteFile(target, src, mode)
// ... later ...
_ = fileProvider.CompensateFileMutation(receipt)
```

`archive.extract` returns the partial stack alongside the error on a mid-extract failure (it already does), so the saga
boundary unwinds it **before** retrying — the undo-before-redo guarantee, with no archive-specific logic.

## Resume

Because the receipt self-declares a *registered* companion, resume reconstructs it the same way it reconstructs any
resource receipt: `reconstructReceipt` reads the concrete type off the compensation companion named on the receipt
(`CompensateFileMutation(receipt *file.Receipt)`), `reflect.New`s a `*file.Receipt`, and `RestoreEncoded` rehydrates its
`resource`/`recoveryID`/`boundary` against the ledger. An `archive.extract` child rebuilds as a `*file.Receipt` from its
**compensation** action even though its dispatch action was `archive.extract`. So `stack.Unwind()` works after a reload,
not only in process.

## Decisions (settled 2026-06-27)

1. **Explicit kind.** The receipt records `create|update|delete` (file) or `create|delete` (dir) in one field, rather
   than inferring it from recovery-id-presence. `CompensateFileMutation` dispatches on it. Self-documenting in the trace
   and unambiguous on resume, at the cost of one field.
2. **`remove`/`remove_all`/`unlink` already route to `CompensateFileMutation` — no `DeleteFile`.** A delete is the
   mirror of a write, so it shares the one undo — but via the existing `Remove`/`RemoveAll`/`Unlink`, whose receipts
   slice 2 already routes to `CompensateFileMutation`. The shapes follow the universal rule: a **single** mutation
   returns one receipt (`remove`, `unlink`); a **batch** returns a `*RecoveryStack` (`remove_all` loops its per-entry
   deletes, like `archive.extract` loops `WriteFile`). One caveat: `unlink`'s subject is a symlink, whose "content" is
   its target — recovery must archive the link itself, not its referent.
3. **Directories are in scope — via the existing `Mkdir`, not a new `MakeDir`.** Directory create/remove are first-class
   mutations producing the same self-describing receipt (`MutationCreateDir`/`DeleteDir`), but `Mkdir` already creates a
   directory and (post-slice-2) returns that receipt, so `archive.extract`'s dir entries call `Mkdir`. The **boundary**
   still handles directories *incidental* to a file write (a file's missing parents); explicit directory entries get
   their `MutationCreateDir` receipt from `Mkdir`. The `MutationDeleteDir` kind and its `compensateRemoveDir` arm remain
   the undo vocabulary (recreating a removed dir) regardless of which forward op records a dir deletion. Known gap to
   track: a `remove_all` over a subtree with a truly **empty** directory must record that dir so it round-trips
   (`remove_all` archives the subtree wholesale today).
4. **Constructor-stamped compensation identity; two fields, two roles.** The receipt carries `forwardAction` (the
   provider method that dispatched — audit) and `compensatingAction` (the compensator identity — the undo). The
   receipt's **constructor** stamps `compensatingAction` (`file.NewReceipt` / `NewReceiptWithBoundary` → file's
   compensator), because the type knows its undo; `Commit` stamps `forwardAction` from the dispatching unit and falls
   back to stamping `compensatingAction` from the unit only when the constructor left it empty (a dispatch-derived
   fallback, correct where dispatcher == creator, kept permanently — decision 9). The two were one field
   (`action` / `actionPath`) carrying two jobs; the rename makes the roles explicit and retires the now-misleading
   "Path" (short-vs-canonical) framing.
5. **No announce-exclusion mechanism needed for this surface (supersedes the earlier `+devlore:internal` vs.
   step-24-discriminator analysis).** The earlier framing assumed four exported mutation methods, three of which had to
   be kept off the Starlark surface — which is what raised the `+devlore:internal` flag and the activation-record-first
   discriminator. Inspection (2026-06-29) settled it differently: `DeleteFile`/`MakeDir`/`RemoveDir` are redundant
   synonyms with no caller and no shared logic to factor (decision 8), so they are dropped, not hidden. The one method
   added — `WriteFile` — is a legitimate **public** action: its `io.Reader` is the consumer end of a first-class
   `stream.Resource`, and `op.Convert` already projects `io.Reader` sources (the json/yaml resource constructors do this
   today). So nothing here needs hiding. `+devlore:internal` remains a sound general capability should a genuinely
   Go-only exported provider method appear later, but it is not a prerequisite for slice 4, and step 24 (the
   activation-record-first discriminator) is decoupled from it — sequence step 24 into step 26.
6. **Env comes from the resource, not the provider (step 26 shape).** `WriteFile` takes a `*Resource` (and
   `CompensateFileMutation` a receipt that holds one); the resource is the env-bearer —
   `target.RuntimeEnvironment()` / `receipt.Resource().RuntimeEnvironment()` off-dispatch, `activation.RuntimeEnvironment`
   for the action wrappers. This is the step-26 split (providers go stateless, resources keep the env), and is why it
   takes `*Resource` rather than `(activation, path)`. Until step 26, it rides the still-present `p.RuntimeEnvironment()`
   helpers (option 1 — functionally identical today, provider-env ≡ resource-env in one session); step 26 flips the file
   provider's env-source wholesale and it reaches its final phase-8 shape.
7. **Compensation resolves via a compensator-name index.** Registration indexes every `Compensate*` method by its
   dotted name; the compensation lookup resolves the receipt's `compensatingAction` through that index, falling back to
   the existing forward→`.undo` path for dispatch-action values (dispatcher-equals-creator providers). The lookup gains one
   branch; the receipt plumbing and `Commit` carry the rest. File's per-action `Compensate` companions collapse into
   `CompensateFileMutation`, and registration's "a compensable forward requires a `Compensate<Name>` companion" check
   relaxes accordingly — compensation resolves from the receipt, not the name convention.
8. **The four mutation methods collapse to one — `WriteFile` (settled 2026-06-29; supersedes decisions 2, 3, 5 where
   they described four methods).** A "core" method earns its existence only by factoring out non-trivial shared logic.
   `WriteFile` does — streaming `io.Copy` + displacement-to-recovery, reused by `WriteText`/`WriteBytes` and
   `archive.extract` — and it adds a capability the existing surface lacks (streaming from a `stream.Resource`), so it is
   added and **exposed**. `DeleteFile`/`MakeDir`/`RemoveDir` do not: each is an `os`-call-plus-receipt wrapper identical
   to the existing `Remove`/`RemoveAll`/`Mkdir` (which already self-describe post-slice-2), and none has a caller —
   `archive.extract` only *creates* (it calls `WriteFile` + `Mkdir`); compensation does its os operations directly (it
   never calls the forward methods); and the public deletes are `file.remove`/`file.remove_all`. So they are dropped, not
   built (greenfield: when in doubt, delete). `WriteFile` (slice 4) is the one method added; the `archive.extract`
   rewrite is slice 5 — not four methods + a codegen directive.
9. **The `Commit` fallback is permanent, not transitional (settled 2026-06-29; retires slice 6).** `Commit` stamping
   `compensatingAction` = the dispatch action when the constructor left it empty is **correct for every provider where
   the dispatcher is the creator** — `git.Clone`→`CompensateClone`, `service.Enable`→`CompensateEnable`,
   `flow.Subgraph`→`CompensateSubgraph`, `elevator.Elevate`→`CompensateElevate`, all of them. The fallback was only ever
   wrong for a **cross-dispatcher** receipt — `archive.extract` building a `file` receipt — and that is exactly what
   slices 1–5 fixed (a file receipt names `file.compensate_file_mutation` at construction, independent of who dispatches
   it). So after slice 5 the fallback is correct and harmless for every remaining user; dropping it would buy zero
   correctness and force a combinator-compensation re-architecture (`flow`/`elevator` have no resource receipt to stamp —
   their complement is a `*RecoveryStack`). Cost without benefit. The fallback stays; slice 6 is retired and the
   mechanism is complete at slice 5.

## Implementation status (2026-06-29)

Slices 1 and 2 are committed. The originally-planned slice-1 items 4–5 (the mutation methods + `CompensateFileMutation`)
were re-sliced so each lands with its consumer: `CompensateFileMutation` shipped in slice 2, and the one surviving
method `WriteFile` ships in slice 4 (decision 8). **Slice 2 (file seam) — committed:** the field rename
(`forwardAction` / `compensatingAction`), the `Commit` split + separate `compensating_action` serialization, the
compensator-name index (+ relaxed `Compensate<Name>` registration in both `receiver_type` and `method.go`), and
`CompensateFileMutation` (with its `compensateWrite` / `compensateMakeDir` / `compensateMove` / `compensateRemoveDir`
arms). The file constructors now build through the fluent **`ReceiptSpec`** (`NewReceiptSpec(resource, kind)`
+ `WithBoundary` / `WithRecovery` / `WithSource` → `NewReceipt(spec)`, mirroring `op.NodeSpec`/`op.NewNode`), so `kind`
and the compensator identity are fixed at construction — `SetKind` / `SetRecoveryID` / `SetRecoveryDigest` / `SetSource`
are gone. The 10 per-action file companions collapsed into the one `CompensateFileMutation` (a file receipt routes there
via its constructor-stamped `compensatingAction`, with Move detected by the recorded source); `CompensateWalkTree` stays.
The migrated file tests pass. **The archive adaptation shipped with slice 2:** the one-call-site change to
`archive/provider.go` (onto the new file API, so the build stayed green when `file.NewReceiptWithBoundary` was removed)
committed *with* slice 2 — the `inventory`/`devlore-test`/`star` cascade is already resolved. **Slice 3 (the seam tests) is implemented**
(`pkg/op/inventory/seam_test.go`, `file/seam_test.go`); slices 4 (`WriteFile`) and 5 (the `archive.extract` rewrite) are also implemented. **The mechanism is complete.** Slice 6 (dropping the `Commit` fallback) is **retired** — the
fallback is permanent (decision 9).

**Verification model — read this first.** Work in the worktree `devlore-cli.extract-starlark-from-op`, **not** the
sibling `devlore-cli` checkout (building there silently verifies nothing). `make build` compiles `lore`+`star` (which
depend on `pkg/op` + `file`), then fails at `cmd/writ` — the **standing break** (`op.ImmediateOf` /
`plan.Provider.Assemble` undefined). Treat a build that fails *only* at `cmd/writ` as clean. `make test` runs the suite;
the **standing** failures to ignore are `TestBackup_DefaultSuffix` (file), `TestWalkTree_Planned` (devlore-test),
`TestShellCompletionPath_PerShell` (star/cli), plus the `cmd/writ` / `internal/e2e` / docgen build breaks. Anything else
red is new and yours.

**Done — verified (compiles; zero new test failures):**

1. **Streaming write core.** `Provider.write` (`file/provider.go`) takes `src io.Reader` and copies with `io.Copy`;
   `WriteText`/`WriteBytes` wrap their content in `strings.NewReader`.
2. **`kind` field.** `file.Receipt` (`file/receipt.go`) gains `kind MutationKind` (`MutationCreateFile`/`UpdateFile`/
   `DeleteFile`/`CreateDir`/`DeleteDir`) with `Kind()`/`SetKind()`, serialized as `kind` in `MarshalYAML`, read in
   `RestoreEncoded`.
3. **Write kind recorded.** `prepareWrite` (`file/provider.go`) stamps `MutationCreateFile` (target absent) or
   `MutationUpdateFile` (target present → prior archived to recovery) on the receipt, so `write`/`Copy`/`Move` writes
   carry their kind.

**Slice 1 closes at items 1–3** (above). The originally-planned slice-1 items 4–5 — the mutation method(s) and
`CompensateFileMutation` — were **re-sliced**: `CompensateFileMutation` landed with the seam it serves (slice 2), and
the one surviving method `WriteFile` is slice 4; the `archive.extract` rewrite that calls it is slice 5. `DeleteFile`/`MakeDir`/
`RemoveDir` are **not built at all** — redundant synonyms with no caller (decision 8). `WriteFile` waits for its consumer
rather than landing as dead scaffolding (exported-but-uncalled trips the `unused` linter).

**Slice 2 — the compensation seam (file + framework), plus `CompensateFileMutation` (committed).** The receipt names its own undo.

- **Rename the two fields** on `op.ReceiptBase`: `action` → `forwardAction` (dispatch/audit), `actionPath` →
  `compensatingAction` (the undo identity) — getters, the `Receipt` interface, and the serialized envelope keys follow
  (greenfield: rename the wire keys too).
- **`Commit` splits the stamping** (`pkg/op/receipt.go:360`): always stamp `forwardAction` from the dispatching unit;
  stamp `compensatingAction` from the unit **only when the constructor left it empty** (the permanent dispatch-derived fallback — decision 9).
- **`file.NewReceipt` / `NewReceiptWithBoundary` stamp `compensatingAction`** = file's compensator identity
  (`file/receipt.go:87`, `:104`) — every `*file.Receipt` is born naming its undo, archive's included.
- **Compensator-name index** built at registration (`pkg/op/receiver_type.go`): every `Compensate*` method by its
  dotted name. `invokeCompensateForReceipt` / `receiptTypeForAction` resolve `compensatingAction` through it, falling
  back to the existing forward→`.undo` path (`pkg/op/recovery_stack.go:559`, `:685`). Relax the "compensable forward
  requires a `Compensate<Name>` companion" check (`pkg/op/receiver_type.go:596`) so compensation resolves from the
  receipt.
- **Serialize `compensatingAction` separately** in the recovery-stack envelope (`receiptEnvelope`
  `pkg/op/recovery_stack.go:386`, `recoveryEntryData` `:419`); `fromEntries` restores `forwardAction` /
  `compensatingAction` independently (drop the `ActionPath := Action` aliasing at `:503`).
- **`CompensateFileMutation(receipt *file.Receipt)`** — generalize `compensateWrite` (`file/provider.go:1306`) to
  dispatch on `receipt.Kind()`: files → the existing `compensateWrite` body; `MutationCreateDir` → remove the created
  dir (reuse `CompensateMkdir`'s upward walk); `MutationDeleteDir` → recreate it. File's per-action `Compensate`
  companions collapse into it. Env comes from `receipt.Resource()` (decision 6).

Non-file compensable providers are **untouched** in slice 2 — `Commit`'s fallback stamps their `compensatingAction` =
the dispatch action, which resolves via the forward→`.undo` path exactly as today.

**Slice 3 — the seam tests (implemented).** Prove the seam end-to-end with **no production code**. The motivating gap:
every existing file compensation test calls `p.CompensateFileMutation(receipt)` *directly* (e.g.
`file/provider_test.go:264`), and the inventory test only checks `CompensatorByName(...)` resolves — **nothing exercised
the real path**, `stack.Push(receipt)` → `stack.Unwind()` resolving the constructor-stamped `compensatingAction` through
the compensator-name index to `CompensateFileMutation`. Four tests, two packages:

*Group A — the seam end-to-end (`pkg/op/inventory/seam_test.go`, where the gen blank-imports populate the registry):*

1. **`TestRecoveryStackUnwind_FileReceiptCreate_RemovesViaIndex`** — intern a `file.Resource` over a real temp file,
   `NewReceipt(NewReceiptSpec(resource, MutationCreateFile))`, `Commit` (self-complement), `Push` then `Unwind`, assert
   the file is removed. Asserts the decoupling directly: `CompensatingAction()` is `file.compensate_file_mutation` while
   `ForwardAction()` is empty — compensation follows the constructor-stamped compensator, **not** the dispatcher, which
   is exactly why an `archive.extract`-built receipt compensates as a file mutation.
2. **`TestRecoveryStackUnwind_FileReceiptUpdate_RestoresViaIndex`** — the same path for an *update*: prior content
   archived to recovery, the overwrite displaced it, `Unwind` restores it. Covers the displaced-content arm through the
   index — the path archive relies on for files it overwrites.

   (The earlier "non-file dispatcher" variant — stamping `forwardAction = archive.extract` — is **not** a standalone
   unit test: `ExecutableUnit` carries unexported methods, so a fake dispatching unit can't be built outside `op`. A1's
   empty `forwardAction` already proves compensation does not depend on a file dispatcher; the literal cross-dispatcher
   case is proven by slice 5's `archive.extract` round-trip.)

*Group B — survives a reload: not a standalone test.* The save → load → compensate path for a file receipt is already
covered by `plan`'s `TestGraphResumeThenFail_RollsBack_ViaPublicAPI` (a pre-pause `mkdir` rolls back after the trace is
saved, reloaded, and resumed). A focused inventory test can't add the cross-dispatcher variant: the reload's `rearm`
pass is unexported on `op.RecoveryStack`, and `op` cannot import `file` (cycle), so the round-trip can only be driven
through the lifecycle harness. The cross-dispatcher serialization proof rides slice 5's archive round-trip.

*Group C — `CompensateFileMutation` dispatch arms (`file/seam_test.go`):*

3. **`TestCompensateFileMutation_UnknownKind_Errors`** — an unrecognized `Kind` hits the `default:` arm and errors.
4. **`TestCompensateFileMutation_DeleteDir_RecreatesDir`** — a `MutationDeleteDir` receipt for a removed dir; asserts
   `compensateRemoveDir` recreates it. This arm has no forward producer (decision 8 dropped `RemoveDir`), so this is its
   only coverage.

Verified: `make test` — `pkg/op/inventory` green; `file`'s only failure is the standing `TestBackup_DefaultSuffix`. The
`MutationDeleteFile` restore arm is covered by the migrated `TestCompensateLink`/`Copy`/`Backup`. Closes slice 2's test
debt.

**Slice 4 — `WriteFile`, the exported streaming-write method (implemented).** A thin exported wrapper over the slice-1 streaming core:

```go
func (p *Provider) WriteFile(target *Resource, src io.Reader, mode os.FileMode) (*Resource, *Receipt, error) {
	return p.write(target, src, mode, "")
}
```

`p.write` already streams via `io.Copy` and builds the self-describing receipt through `prepareWrite` (slice 1), so
`WriteFile` adds almost nothing — it exposes that core on a resource-typed signature. It takes an already-interned
`target` (env + producer ride the resource, decision 6), so no activation record and no ownership change.
`WriteText`/`WriteBytes` keep their `chown` via the shared `write` core (rewrapping onto `WriteFile` would drop it —
`WriteFile` is the no-chown streaming export). It is announced as `file.write_file` (public, decisions
5/8); Go callers (`archive`) use it directly, and the Starlark path becomes callable once `stream.Resource` supplies an
`io.Reader` source — until then a `file.write_file` Starlark call errors *cleanly* at `op.Convert` (no `io.Reader`
source converter is registered yet); it does not panic. **Tests:** create (new file → removed on compensate) and update
(overwrite → prior restored) round-trips through `CompensateFileMutation`; `WriteText`/`WriteBytes` still pass via the
delegation. File-provider work; needs only slices 1–2.

**Slice 5 — the `archive.extract` rewrite (onto `WriteFile` + `Mkdir`; fixes #277) — implemented. Absorbed by the
[archive-provider plan](archive-provider.md).** Replace archive's hand-built receipts and os-level writes with a loop: a
dir entry calls the existing `Mkdir` (a `MutationCreateDir` receipt); a file entry calls
`fileProvider.WriteFile(target, entry.Reader, mode)`. Because `WriteFile` (via `prepareWrite`) archives any displaced
content and stamps the `MutationUpdateFile` kind + recovery onto the receipt, **#277 closes for free** — archive's
overwrite-compensation now restores prior content (today a no-op: archive records `PriorArchiveID` but never threads it
onto the receipt). `Extract` pushes each receipt onto one `*op.RecoveryStack` and returns it; a mid-extract failure
returns the partial stack (undo-before-redo); `CompensateExtract` collapses to `stack.Unwind()`. archive's
`writeExtractedFile` / `extractedEntry` / per-format write logic are deleted (the entry iterator is `openArchive`; its
content-detection is the concurrent session's, after this). **Tests** (rewriting the build-broken
`archive/provider_test.go` to the `*RecoveryStack` signature): extract→compensate round-trips for **new files**,
**displaced files** (the #277 proof — prior content restored), and **directory entries**; a **mid-extract failure**
unwinds the partial stack; plus the revised tar.gz / zip / zip-slip / unsupported-format cases. Depends on slice 4;
lands in the archive plan's S1/S2.

**Slice 6 — retired (2026-06-29).** The plan called for migrating the other compensable providers off the `Commit`
fallback, then dropping it. Decision 9 retires this: the fallback is correct for every dispatcher-equals-creator provider
(which is all the remaining ones — `git`/`service`/`encryption`/`pkg`/`flow`/`elevator` each compensate their own
receipts), so dropping it buys zero correctness, and `flow`/`elevator` (combinators with no resource receipt) cannot
stamp a compensator without a framework re-architecture. The fallback stays; the file-mutation mechanism is **complete at
slice 5**.

**Still uncommitted:** `archive`'s broken `provider_test.go` (it still uses the pre-rewrite `[]op.Receipt` shape — the
standing `archive [build failed]`), left for the [archive-provider plan](archive-provider.md) (slice 5) to rewrite
alongside the production rewrite. `archive/provider.go` itself committed with slice 2.
