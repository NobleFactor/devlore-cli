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

- **Four forward operations, one core.** A filesystem-node mutation: create/update/delete a **file**
  (`WriteFile`/`DeleteFile`) and create/delete a **directory** (`MakeDir`/`RemoveDir`). The file pair lifts the existing
  `p.write` → `prepareWrite` core (which archives the prior to recovery and builds the receipt); the dir pair is new but
  small. All four are exported and dispatch-independent, so `archive.extract`, `remove_all`, and any future
  file-mutating provider call the same surface.
- **One self-describing receipt.** It carries everything the undo needs — `resource`, an explicit `kind`
  (`create|update|delete` file, `create|delete` dir), `recoveryID`, `boundary` — *and* names its own undo, stamped by
  the receipt's **constructor**, because the receipt's *type* knows its undo (`*file.Receipt` is always undone by the
  file provider) regardless of which method or dispatcher created it.
- **One undo.** `compensateWrite`, generalized to `CompensateFileMutation`, inverts any of the four by dispatching on
  the receipt's `kind`. `Unwind` routes to it via the receipt's compensation identity.

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
  providers stamp it; see slice 2b).

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

// DeleteFile removes the file at target, archiving its prior content to the recovery site, and returns a
// self-describing *Receipt whose undo restores it.
func (p *Provider) DeleteFile(target *Resource) (*Receipt, error)

// MakeDir creates the directory at target; its receipt's undo removes the directory the call created.
func (p *Provider) MakeDir(target *Resource, mode os.FileMode) (*Resource, *Receipt, error)

// RemoveDir removes the (empty) directory at target; its receipt's undo recreates it.
func (p *Provider) RemoveDir(target *Resource) (*Receipt, error)

// CompensateFileMutation inverts any of the four operations, dispatching on the receipt's kind: remove a created
// file or directory, restore prior content from recovery for an update or delete, recreate a removed directory, and
// prune directories the mutation created. This is today's compensateWrite, generalized to the single mutation undo
// and named directly by every mutation receipt.
func (p *Provider) CompensateFileMutation(receipt *Receipt) error
```

`WriteFile`/`DeleteFile` are `p.write`/`Remove` with two changes: `p.write` takes `src io.Reader` and streams it with
`io.Copy` (replacing the `[]byte` write — true zero-copy via the kernel fast path for file-to-file, constant-memory
streaming for a decompressed archive entry), and every mutation receipt self-declares `CompensateFileMutation` as its
compensation companion. `MakeDir`/`RemoveDir` are the directory pair `archive.extract` and `remove_all` need for
standalone and empty directories; the boundary still handles directories incidental to a file write.

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

		// A directory entry makes the dir; a file entry streams straight to disk — the same writes file.mkdir and
		// file.write_bytes perform, no full-content buffer. Either way, one self-describing receipt.
		var receipt *file.Receipt
		if entry.IsDir {
			_, receipt, err = fileProvider.MakeDir(target, entry.Mode)
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
2. **`remove`/`remove_all`/`unlink` fold in.** A delete is the mirror of a write, so they share `DeleteFile` +
   `CompensateFileMutation`. The shapes follow the universal rule: a **single** mutation returns one receipt
   (`remove`, `unlink`); a **batch** returns a `*RecoveryStack` (`remove_all` loops `DeleteFile`/`RemoveDir`, exactly
   like `archive.extract` loops `WriteFile`/`MakeDir`). One caveat to handle: `unlink`'s subject is a symlink, whose
   "content" is its target — recovery must archive the link itself, not its referent.
3. **Directories are in scope.** `MakeDir`/`RemoveDir` are first-class filesystem-node mutations producing the same
   self-describing receipt, because `archive.extract` (explicit and empty dir entries) and `remove_all` (subtree dirs,
   including empties) require creating and removing standalone directories and undoing them. The **boundary** still
   handles directories *incidental* to a file write (a file's missing parents); explicit directory entries get their
   own receipts. Known gap to track: a `remove_all` over a subtree with a truly **empty** directory — the per-file
   restores recreate parent dirs via the boundary, but an empty dir needs its own `RemoveDir` receipt to round-trip.
4. **Constructor-stamped compensation identity; two fields, two roles.** The receipt carries `forwardAction` (the
   provider method that dispatched — audit) and `compensatingAction` (the compensator identity — the undo). The
   receipt's **constructor** stamps `compensatingAction` (`file.NewReceipt` / `NewReceiptWithBoundary` → file's
   compensator), because the type knows its undo; `Commit` stamps `forwardAction` from the dispatching unit and falls
   back to stamping `compensatingAction` from the unit only when the constructor left it empty (a transitional
   dispatch-derived fallback, correct where dispatcher == creator, removed at slice 2b). The two were one field
   (`action` / `actionPath`) carrying two jobs; the rename makes the roles explicit and retires the now-misleading
   "Path" (short-vs-canonical) framing.
5. **No `+devlore:internal` flag — the activation-record-first discriminator (step 24) is the announce signal.** The four
   mutation methods carry no `*op.ActivationRecord` (they are mechanism, not dispatchable actions), so step 24's
   invariant — *announced ⟺ activation-record-first* — already excludes them from the Starlark surface; a dedicated
   opt-out flag would only re-encode the same fact. Caveat: `generate.star`'s `filter_methods` cannot flip to that
   discriminator until step 24 gives the ~17 existing activation-record-free file actions
   (`Remove`/`Exists`/`Find`/getters/`WalkTree`/…) their leading activation record — so the discriminator is gated on
   step 24, and these methods are not added as exported scaffolding ahead of it.
6. **Env comes from the resource, not the provider (step 26 shape).** Each mutation method takes a `*Resource` (and
   `CompensateFileMutation` a receipt that holds one); the resource is the env-bearer —
   `target.RuntimeEnvironment()` / `receipt.Resource().RuntimeEnvironment()` off-dispatch, `activation.RuntimeEnvironment`
   for the action wrappers. This is the step-26 split (providers go stateless, resources keep the env), and is why these
   take `*Resource` rather than `(activation, path)`. Until step 26, they ride the still-present `p.RuntimeEnvironment()`
   helpers (option 1 — functionally identical today, provider-env ≡ resource-env in one session); step 26 flips the file
   provider's env-source wholesale and these reach their final phase-8 shape.
7. **Compensation resolves via a compensator-name index.** Registration indexes every `Compensate*` method by its
   dotted name; the compensation lookup resolves the receipt's `compensatingAction` through that index, falling back to
   the existing forward→`.undo` path for dispatch-action values (not-yet-migrated providers). The lookup gains one
   branch; the receipt plumbing and `Commit` carry the rest. File's per-action `Compensate` companions collapse into
   `CompensateFileMutation`, and registration's "a compensable forward requires a `Compensate<Name>` companion" check
   relaxes accordingly — compensation resolves from the receipt, not the name convention.

## Implementation status (2026-06-27)

Slice 1's mutation-core items 1–3 are landed and verified (committed). The originally-planned slice-1 items 4–5 are
re-sliced into slices 2–3 so each lands with its consumer (see below). **Slice 2 is in progress:** the field rename
(`forwardAction` / `compensatingAction`), the `Commit` split + separate `compensating_action` serialization, and the
compensator-name index (+ relaxed `Compensate<Name>` registration) have landed and verified; `CompensateFileMutation` +
the file companion collapse + the constructor stamping remain. Slice 3 is not started.

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

**Slice 1 closes at items 1–3** (above). The originally-planned slice-1 items 4–5 — the four exported mutation methods
and `CompensateFileMutation` — are **re-sliced into slices 2–3**, because they have no consumer until the seam routes to
`CompensateFileMutation` (slice 2) and the archive rewrite calls the four methods (slice 3). Adding them now would be
dead scaffolding: exported, they would need the rejected `+devlore:internal` flag to dodge announcement (decision 5);
unexported, they would trip the `unused` linter. Each lands with its consumer.

**Slice 2 — the compensation seam (file + framework), plus `CompensateFileMutation`.** The receipt names its own undo.

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

**Slice 2b — finish the cross-provider seam.** Migrate each remaining compensable provider's receipt constructor to
stamp its `compensatingAction` (git, service, encryption, pkg, elevator, flow), collapsing per-provider companions as
each is done, then **drop the `Commit` fallback** so every receipt declares its compensator explicitly.

**Slice 3 — the archive rewrite, plus the exported mutation surface it consumes. Absorbed by the
[archive-provider plan](archive-provider.md) (2026-06-28).** That plan now owns the exported `file.Provider` mutation
surface (`WriteFile`/`DeleteFile`/`MakeDir`/`RemoveDir`, gated on step 24), the `archive.extract` rewrite onto that
surface, and — net-new — content-based format detection and the decompressor pipeline (the full tar family + zip). See
the [archive-provider plan](archive-provider.md) for its slices and status; the prerequisites here are slices 1–2(b).

**Uncommitted WIP, deliberately *not* in the slice-1 commit:** `archive/provider.go` (interim `*RecoveryStack` shape,
no-op rollback) and its broken `provider_test.go`, left out so the commit stays green; the
[archive-provider plan](archive-provider.md) rewrites both.
