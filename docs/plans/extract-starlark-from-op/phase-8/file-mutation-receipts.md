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
  empty** — a transitional fallback that is correct for every provider where dispatcher == creator (removed once all
  providers stamp it; see slice 5).

Resolution stays a *registered* lookup (so it survives a `Trace` reload — a captured closure does not), now keyed on
`compensatingAction`: registration builds a **compensator-name index** (every `Compensate*` method by its dotted name),
and the compensation lookup resolves `compensatingAction` through it, falling back to the existing forward→`.undo` path
for receipts whose `compensatingAction` is a dispatch action (the not-yet-migrated providers). The receipt declares its
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
   back to stamping `compensatingAction` from the unit only when the constructor left it empty (a transitional
   dispatch-derived fallback, correct where dispatcher == creator, removed at slice 5). The two were one field
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
   the existing forward→`.undo` path for dispatch-action values (not-yet-migrated providers). The lookup gains one
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
   built (greenfield: when in doubt, delete). Slice 4 is therefore **one new method (`WriteFile`) + the archive
   rewrite**, not four methods + a codegen directive.

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
committed *with* slice 2 — the `inventory`/`devlore-test`/`star` cascade is already resolved. Slices **3** (seam tests),
**4** (`WriteFile` + the archive rewrite), and **5** (cross-provider migration + dropping the `Commit` fallback) remain.

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
the one surviving method `WriteFile` lands with the archive rewrite that calls it (slice 4). `DeleteFile`/`MakeDir`/
`RemoveDir` are **not built at all** — redundant synonyms with no caller (decision 8). `WriteFile` waits for its consumer
rather than landing as dead scaffolding (exported-but-uncalled trips the `unused` linter).

**Slice 2 — the compensation seam (file + framework), plus `CompensateFileMutation` (committed).** The receipt names its own undo.

- **Rename the two fields** on `op.ReceiptBase`: `action` → `forwardAction` (dispatch/audit), `actionPath` →
  `compensatingAction` (the undo identity) — getters, the `Receipt` interface, and the serialized envelope keys follow
  (greenfield: rename the wire keys too).
- **`Commit` splits the stamping** (`pkg/op/receipt.go:360`): always stamp `forwardAction` from the dispatching unit;
  stamp `compensatingAction` from the unit **only when the constructor left it empty** (transitional fallback).
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

**Slice 3 — the seam tests.** Prove the seam end-to-end with no production code: a non-file dispatcher's `*file.Receipt`
routes to `CompensateFileMutation` via the compensator-name index; a save → load → unwind round-trip preserves
`compensatingAction` and still compensates after reload; and `CompensateFileMutation`'s unknown-kind arm errors. Closes
slice 2's test debt.

**Slice 4 — the archive rewrite, plus the one exported mutation method it consumes. Absorbed by the
[archive-provider plan](archive-provider.md) (2026-06-28).** That plan owns the exported `file.Provider.WriteFile`
method (a public streaming-write action — no step-24 gate, no exclusion marker; decisions 5, 8), the `archive.extract`
rewrite onto `WriteFile` + the existing `Mkdir`, and — net-new — content-based format detection and the decompressor
pipeline (the full tar family + zip). See the [archive-provider plan](archive-provider.md) for its slices and status;
the prerequisites here are slices 1–2 (archive builds file receipts, which are constructor-stamped as of slice 2, so it
does not wait on the cross-provider migration).

**Slice 5 — finish the cross-provider seam.** Migrate each remaining compensable provider's receipt constructor to
stamp its `compensatingAction` (git, service, encryption, pkg, elevator, flow), collapsing per-provider companions as
each is done, then **drop the `Commit` fallback** so every receipt declares its compensator explicitly. Last, because
the fallback can only go once every provider and consumer is off it.

**Still uncommitted:** `archive`'s broken `provider_test.go` (it still uses the pre-rewrite `[]op.Receipt` shape — the
standing `archive [build failed]`), left for the [archive-provider plan](archive-provider.md) (slice 4) to rewrite
alongside the production rewrite. `archive/provider.go` itself committed with slice 2.
