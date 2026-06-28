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
  (`create|update|delete` file, `create|delete` dir), `recoveryID`, `boundary` — *and* names its own undo companion,
  stamped by the operation when it builds the receipt, because the operation knows what it is regardless of caller.
- **One undo.** `compensateWrite`, generalized to `CompensateFileMutation`, inverts any of the four by dispatching on
  the receipt's `kind`. `Unwind` routes to it via the receipt's named compensation companion.

### The seam — a receipt always declares its own compensation

`Unwind` and resume already resolve a *registered* `Compensate` companion (so it survives a `Trace` reload — a captured
closure does not). We keep that, and change **where the companion's identity comes from**:

- `ReceiptBase` gains a **compensation action** that the producing operation **always sets** — no default, no
  dispatch-derived fallback. `WriteFile`/`DeleteFile`/`MakeDir`/`RemoveDir` stamp it when they build the receipt.
- `RecoveryStack.Push` and resume's `reconstructReceipt` route by the receipt's compensation action.
- Because the receipt names its undo directly, the per-action `Compensate` companions
  (`CompensateWriteText`/`WriteBytes`/`Remove`/`RemoveAll`/`Unlink`/`Mkdir`) collapse into the single
  `CompensateFileMutation`. The dispatch action stays on the receipt only for the **audit trail**.

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
4. **Always-set compensation action — no default, no override.** Every mutation operation stamps the receipt's
   compensation companion (`CompensateFileMutation`) when it builds it. There is no dispatch-derived default and no
   override path; the framework routes by what the receipt declares. This is what collapses the per-action `Compensate`
   companions into one, and it changes nothing observable for dispatched ops (the undo is identical either way).

## Implementation status (2026-06-27)

Slice 1 (the file-provider mutation core) is partially landed and verified; slices 2–3 are not started.

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

**Next — slice 1, remaining:**

4. **Exported mutation surface** (`file/provider.go`): `WriteFile(target *Resource, src io.Reader, mode os.FileMode)`
   (wrap `write`, chown `""`), `DeleteFile(target)` (wrap `Remove`, set `MutationDeleteFile`), `MakeDir(target, mode)`
   (wrap `Mkdir`, set `MutationCreateDir`), `RemoveDir(target)` (set `MutationDeleteDir`). This is the surface archive
   calls.
5. **`CompensateFileMutation(receipt)`** — generalize `compensateWrite` (`file/provider.go:1306`, already inverts file
   create/update/delete via recovery-id-presence + the boundary walk) to dispatch on `receipt.Kind()`: files → the
   existing `compensateWrite` body; `MutationCreateDir` → remove the created dir (reuse `CompensateMkdir`'s upward
   walk); `MutationDeleteDir` → recreate it.

**Then:**

- **Slice 2 — the seam (cross-provider; the big one).** `op.ReceiptBase` gains a compensation action the producing op
  always sets (decision 4 — no default, no override); `RecoveryStack.Push` and resume's `reconstructReceipt` route by
  it; **every** compensable provider's forward methods set it (file, git, service, encryption, pkg, elevator, flow), and
  the per-action `Compensate*` companions collapse.
- **Slice 3 — archive rewrite.** `archive.extract` loops `WriteFile`/`MakeDir`, returns the `*RecoveryStack`;
  `CompensateExtract` → `stack.Unwind()`; fix `archive/provider_test.go` (currently `[build failed]` — it still uses
  `len(receipts)` / `range receipts` against the old `[]op.Receipt` signature).

**Uncommitted WIP, deliberately *not* in the slice-1 commit:** `archive/provider.go` (interim `*RecoveryStack` shape,
no-op rollback) and its broken `provider_test.go`, left out so the commit stays green; slice 3 rewrites both.
