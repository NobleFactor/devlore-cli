# Step 7: Replace Hand-Written Ops with Service Delegation

## Context

Steps 1-5 built typed slots, engine slot filling, a single `Operation` interface,
and extracted 5 service structs. Step 6 updated the generator templates. This step
replaces the 31 hand-written ops with delegation ops that call through to services.

Worktree: `devlore-cli.star-gen-receiver` (branch `feat/ops-delegation`)
Files: `internal/execution/ops*.go`

## Key Design Decisions

### No subpackages — generate into `execution` package

Phase 6 plan proposed `generated/fileops/` subpackages. This creates a circular
import: `execution` → `fileops` → `execution` (for Context, Node, Operation).
Instead: flat `_gen.go` files in `execution` package. Nuke-safe: `rm *_gen.go`.

### Keep current flat op names

The generator produces dotted names (`"file.link"`) but plan receivers create
nodes with flat names (`"link"`, `"package-install"`). Changing all names is a
cross-cutting change across 15+ files. Defer to Steps 8-9 when plan receivers
are regenerated — both sides change together.

### Hand-write delegation ops (not running the generator)

Several ops need adaptations the generator can't produce:

| Edge Case | Ops Affected | Handling |
|---|---|---|
| `node.GetMode()` field, not slot | Copy, Write | Read from Node method |
| `ctx.Data` as template data | Render | Copy ctx.Data map |
| `node.GetProject()` method | Render | Read from Node method |
| Backup return → annotations | Backup | Store in node.Annotations |
| `prune_empty_dirs` key mismatch | Unlink, Remove | Use ctx.Data key as slot name |
| `map[string]func() error` not in type map | Validate | Keep hand-written |

Hand-writing follows the template patterns exactly, enabling later regeneration.

## Changes

### New files

**`ops_file_gen.go`** — 8 ops from FileService (`file_service.go`)

| Op Struct | Name() | Content Model | Special |
|---|---|---|---|
| FileLinkOp | `link` | none | |
| FileCopyOp | `copy` | consumer | mode from GetMode() |
| FileRenderOp | `render` | transformer | templateData from ctx.Data, project from GetProject() |
| FileBackupOp | `backup` | none | return value → Annotations["backup_path"] |
| FileUnlinkOp | `unlink` | none | prune from slot "prune_empty_dirs" |
| FileRemoveOp | `remove` | none | prune from slot "prune_empty_dirs" |
| FileWriteOp | `write` | none | mode from GetMode() |
| FileMoveOp | `move` | none | |

Registration: `FileOps(impl *FileService) []Operation`

**`ops_encryption_gen.go`** — 1 op from EncryptionService (`encryption_service.go`)

| Op Struct | Name() | Content Model |
|---|---|---|
| EncryptionDecryptOp | `decrypt` | transformer |

Registration: `EncryptionOps(impl *EncryptionService) []Operation`

**`ops_package_gen.go`** — 4 ops from PackageService (`package_service.go`)

| Op Struct | Name() | Content Model |
|---|---|---|
| PackageInstallOp | `package-install` | none |
| PackageUpgradeOp | `package-upgrade` | none |
| PackageRemoveOp | `package-remove` | none |
| PackageUpdateOp | `package-update` | none |

Registration: `PackageOps(impl *PackageService) []Operation`

**`ops_shell_gen.go`** — 2 ops from ShellService (`shell_service.go`)

| Op Struct | Name() | Content Model | Notes |
|---|---|---|---|
| ShellOp | `shell` | none | output = ctx.Logger |
| PowerShellOp | `powershell` | none | output = ctx.Logger |

Registration: `ShellOps(impl *ShellService) []Operation`

**`ops_service_manager_gen.go`** — 5 NEW ops from ServiceManagerService

| Op Struct | Name() | Notes |
|---|---|---|
| ServiceManagerStartOp | `service-start` | output = ctx.Logger |
| ServiceManagerStopOp | `service-stop` | output = ctx.Logger |
| ServiceManagerRestartOp | `service-restart` | output = ctx.Logger |
| ServiceManagerEnableOp | `service-enable` | output = ctx.Logger |
| ServiceManagerDisableOp | `service-disable` | output = ctx.Logger |

Names match `plan_root.go:196` (`"service-" + actionStr`). The 15 platform-
specific ops in ops_service.go were never registered in AllOps() — dead code.

Registration: `ServiceManagerOps(impl *ServiceManagerService) []Operation`

**`ops_registry.go`** — AllOps wiring + ValidateOp

ValidateOp (moved from ops.go unchanged): reads `ctx.Data["validators"]`,
type `map[string]func() error` not in generator type mappings.

```go
func AllOps() []Operation {
    var ops []Operation
    ops = append(ops, FileOps(&FileService{})...)
    ops = append(ops, EncryptionOps(&EncryptionService{})...)
    ops = append(ops, PackageOps(&PackageService{})...)
    ops = append(ops, ShellOps(&ShellService{})...)
    ops = append(ops, ServiceManagerOps(&ServiceManagerService{})...)
    ops = append(ops, &ValidateOp{})
    return ops
}
```

### Deleted files

| File | Replaced by |
|---|---|
| `ops.go` | `ops_file_gen.go` + `ops_registry.go` |
| `ops_package.go` | `ops_package_gen.go` + `ops_shell_gen.go` |
| `ops_service.go` | `ops_service_manager_gen.go` |

`parsePackages` removed — typed slots provide `[]string` directly.
All helpers (`resolvePMFor*`, `runBrewCask*`, `pruneParents`, `isSubpath`)
already live in their respective `*_service.go` files.

### Op count

Before: 16 registered (10 file + 6 package) + 15 dead service = 31 total
After: 21 registered (8 file + 1 encryption + 4 package + 2 shell + 5 service_mgr + 1 validate)

### Delegation pattern

Standard (none, error-only):
```go
type FileLinkOp struct{ impl *FileService }

func (o *FileLinkOp) Name() string { return "link" }

func (o *FileLinkOp) Execute(ctx *Context, node *Node) error {
    source := node.GetSlot("source").(string)
    path := node.GetSlot("path").(string)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] link %v %v\n", source, path)
        return nil
    }
    return o.impl.Link(source, path)
}
```

Consumer (Copy — reads content, checksums, discards string return):
```go
func (o *FileCopyOp) Execute(ctx *Context, node *Node) error {
    path := node.GetSlot("path").(string)
    mode := node.GetMode()
    content, err := ctx.ContentFor(node)
    if err != nil { return err }

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] copy %v\n", path)
        ctx.TargetChecksum = ChecksumBytes(content)
        return nil
    }
    _, err = o.impl.Copy(path, mode, content)
    return err
}
```

Transformer (Render — reads content, stores transformed output):
```go
func (o *FileRenderOp) Execute(ctx *Context, node *Node) error {
    source := node.GetSlot("source").(string)
    path := node.GetSlot("path").(string)
    project := node.GetProject()
    templateData := make(map[string]any)
    for k, v := range ctx.Data { templateData[k] = v }
    content, err := ctx.ContentFor(node)
    if err != nil { return err }

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] render %v %v\n", source, path)
        return nil
    }
    result, err := o.impl.Render(templateData, source, path, project, content)
    if err != nil { return err }
    ctx.StoreContent(node, result)
    return nil
}
```

### Slot read patterns for engine-injected params

Engine slot filling (Step 4) copies ctx.Data keys into node slots. Slot reads
use the ctx.Data key name (snake_case), which matches the generator's
`p.SnakeName` pattern for most params:

| Service Param | Slot Key | ctx.Data Key | Match |
|---|---|---|---|
| decryptor | `"decryptor"` | `"decryptor"` | yes |
| backupSuffix | `"backup_suffix"` | `"backup_suffix"` | yes |
| gitMv | `"git_mv"` | `"git_mv"` | yes |
| pruneBoundary | `"prune_boundary"` | `"prune_boundary"` | yes |
| prune | **`"prune_empty_dirs"`** | `"prune_empty_dirs"` | name mismatch (Step 10 renames) |
| io.Writer | n/a | n/a | `ctx.Logger` directly |
| templateData | n/a | n/a | Copy `ctx.Data` map |
| mode | n/a | n/a | `node.GetMode()` |
| project | n/a | n/a | `node.GetProject()` |

## Verification

```bash
cd <worktree>
go build ./...
go test ./internal/execution/ -count=1
go test ./internal/starlark/ -count=1
go test ./internal/writ/ -count=1
go test ./internal/lore/ -count=1
go vet ./...
```
