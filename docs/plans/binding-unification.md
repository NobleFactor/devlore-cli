# Binding Unification: Generate Receivers and Plan Bindings from Providers

## Context

The `internal/execution/provider/` packages are the single source of truth for
resource operations (file, package, service, shell, git, archive, net, encryption,
template, content). Each provider has forward methods and compensation methods.

Three tools share these providers:
- **lore** needs plan bindings (graph construction) and graph actions (graph execution)
- **star** extensions need receivers (immediate execution)
- **writ** needs graph actions (graph execution)

Currently the code generation pipeline (`devlore ops.generate`) produces three outputs
from a Provider struct: `plan_receiver`, `graph_actions`, `realtime_receiver`. But:

1. Only 2 of 10 providers have generated plan bindings (archive, git). The other
   4 plan bindings (file, package, encryption, template) are hand-written.
2. Generated receivers (archive, service) call `host.Host`/`host.ServiceManager`
   instead of Provider methods — they bypass the Provider model entirely.
3. Hand-written receivers (git, package, shell, http) also bypass Providers,
   shelling out via `exec.Command` or calling `host` methods directly.
4. Docker and Npm have no providers at all.

**Result**: Receivers and plan bindings can diverge from each other and from the
Provider. Changes to a Provider don't propagate to either binding layer.

## Design Principles

1. Providers are shared infrastructure. lore, star, and writ each generate what
   they need: receivers (immediate), plan bindings (graph nodes), actions (Do/Undo).
2. Receivers execute Provider methods immediately. No compensation.
3. Plan bindings create graph nodes. Actions execute Provider methods during graph
   traversal with compensation.
4. A receiver call and a plan binding call execute the same Provider method.

## Provider Inventory

| Provider | Methods | Compensable | Plan Binding | Receiver |
|----------|---------|-------------|--------------|----------|
| file | Link, Copy, Write, Remove, Move, Unlink, Mkdir, Backup, Source | 6 of 9 | hand-written | none |
| pkg | Install, Upgrade, Remove, Update | 3 of 4 | hand-written | hand-written (bypasses) |
| service | Start, Stop, Restart, Enable, Disable | 5 of 5 | top-level builtin | generated (bypasses) |
| shell | Shell, PowerShell | 0 of 2 | top-level builtin | hand-written (bypasses) |
| git | Clone, Checkout, Pull | 1 of 3 | generated | hand-written (bypasses) |
| archive | Extract | 1 of 1 | generated | generated (bypasses) |
| net | Download | 1 of 1 | top-level builtin | hand-written (bypasses) |
| encryption | Decrypt | 0 of 1 | hand-written | none |
| template | Render | 0 of 1 | hand-written | none |
| content | Literal | 0 of 1 | top-level builtin | none |

## Phase 1: Fix the realtime_receiver Template

The `realtime_receiver` template (builtin in noblefactor-ops) generates receivers
that call `host.Host` methods. It must generate receivers that call Provider methods.

### Template change

The generated receiver holds a Provider instance and delegates to its methods:

```go
type {{.StructName}}Receiver struct {
    Receiver
    provider *{{.Category}}.Provider
    output   io.Writer
}

func (r *{{.StructName}}Receiver) methodName(...) (starlark.Value, error) {
    result, _, err := r.provider.MethodName(args...)  // ignore compensation receipt
    // convert result to Starlark value
}
```

### Provider constructor dependencies

Some providers need dependencies at call time (host.ServiceManager for service,
host.PackageManager for pkg). Two options:

**A.** Provider holds dependencies as fields. Constructor: `&pkg.Provider{PM: pm}`.
**B.** Dependencies passed per-call. Method: `Install(pm, packages, ...)`.

Current state: Providers are stateless structs (`&Provider{}`). Dependencies like
`host.ServiceManager` are passed per-method in the `actions_gen.go` wrappers via
slots. For receivers, the Provider needs these dependencies at construction time.

**Decision**: Add optional fields to Provider structs for dependencies that receivers
need. Example: `pkg.Provider{PM: host.PackageManager}`. The generated receiver
constructor sets these. Actions continue using slots.

### Files

| Repo | File | Action |
|------|------|--------|
| noblefactor-ops | `internal/starlark/receiver_go_gen.go` | Modify: new `realtimeProviderBody` template helper |
| noblefactor-ops | `internal/starlark/receiver_go_gen_test.go` | Add test for new helper |
| devlore-cli | `star/extensions/com.noblefactor.devlore.Ops/templates/realtime_receiver.go.template` | Create: local template replacing builtin |
| devlore-cli | `star/extensions/com.noblefactor.devlore.Ops/commands/generate.star` | Update LOCAL_TEMPLATES |

## Phase 2: Generate Plan Bindings for All Providers

Replace the 4 hand-written plan files with generated ones. The generated pattern
matches the existing `plan_archive_gen.go` and `plan_git_gen.go`: embed Receiver,
Attr/AttrNames, FillSlot per parameter, return Output.

### Variadic args (PackagePlan)

`pkg.Provider.Install(packages []string, manager string, cask bool)` — the
`packages` parameter is variadic in the Starlark API (`plan.package.install("a", "b")`).
The `planUnpackArgs` template helper needs to handle `[]string` as variadic
positional args, not a single kwarg. This is a noblefactor-ops template change.

### Service and shell become sub-namespaces

Currently `plan.service(name, action)` and `plan.shell(command)` are top-level
builtins on PlanRoot that dispatch by action string. After generation:

- `plan.service.start(name)`, `plan.service.stop(name)`, etc.
- `plan.shell.exec(command)` (matches `shell.Provider.Shell`)
- `plan.net.download(url)` (was `plan.download(url)`)
- `plan.content.literal(content)` (was `plan.literal(content)`)

PlanRoot keeps only `plan.source(path)` and `plan.gather(...)` as top-level
builtins (graph construction primitives, not resource operations).

### Files

| File | Action |
|------|--------|
| `internal/starlark/plan_file_gen.go` | Generate (replaces plan_file.go) |
| `internal/starlark/plan_package_gen.go` | Generate (replaces plan_package.go) |
| `internal/starlark/plan_encryption_gen.go` | Generate (replaces plan_encryption.go) |
| `internal/starlark/plan_template_gen.go` | Generate (replaces plan_template.go) |
| `internal/starlark/plan_service_gen.go` | Generate (new) |
| `internal/starlark/plan_shell_gen.go` | Generate (new) |
| `internal/starlark/plan_net_gen.go` | Generate (new) |
| `internal/starlark/plan_content_gen.go` | Generate (new) |
| `internal/starlark/plan_file.go` | Delete |
| `internal/starlark/plan_package.go` | Delete |
| `internal/starlark/plan_encryption.go` | Delete |
| `internal/starlark/plan_template.go` | Delete |
| `internal/starlark/plan_root.go` | Modify: add service, shell, net, content sub-namespaces; remove service/shell/download/literal builtins |

## Phase 3: Generate Receivers for All Providers

Generate receivers that call Provider methods for all 10 providers. For providers
where the hand-written receiver had additional query/convenience methods beyond
Provider scope, those methods move to a companion `_queries.go` file.

### Query methods

Query methods (read-only introspection) are not Provider operations:

| Receiver | Query methods |
|----------|--------------|
| package | manager(), installed(name), version(name), feature(name), setting(name) |
| git | installed(), version(), repo_root(), current_branch(), remote_url(), is_clean(), latest_tag(), commit_hash() + 14 kwargs pass-through commands |
| shell | which(name) |
| http | get(url) |

These stay hand-written in companion files. The generated `Attr()`/`AttrNames()`
must include them. The generator accepts an `--extra-attrs` flag listing additional
attribute names contributed by the companion file.

### Docker and Npm

Docker (21 methods) and Npm (17 methods) are kwargs pass-through CLI wrappers
with no Provider. They remain hand-written receivers for now. When plan bindings
are needed for them, providers will be created first.

### Files

| File | Action |
|------|--------|
| `internal/starlark/receiver_file_gen.go` | Generate (new) |
| `internal/starlark/receiver_package_gen.go` | Generate (replaces receiver_package.go operations) |
| `internal/starlark/receiver_package_queries.go` | Create: hand-written (manager, installed, version, feature, setting) |
| `internal/starlark/receiver_service_gen.go` | Regenerate (now calls Provider) |
| `internal/starlark/receiver_shell_gen.go` | Generate (replaces receiver_shell.go operations) |
| `internal/starlark/receiver_shell_queries.go` | Create: hand-written (which) |
| `internal/starlark/receiver_git_gen.go` | Generate (replaces receiver_git.go for clone/checkout/pull) |
| `internal/starlark/receiver_git_queries.go` | Create: hand-written (27 query/convenience methods) |
| `internal/starlark/receiver_archive_gen.go` | Regenerate (now calls Provider) |
| `internal/starlark/receiver_net_gen.go` | Generate (replaces receiver_http.go for download) |
| `internal/starlark/receiver_net_queries.go` | Create: hand-written (get) |
| `internal/starlark/receiver_encryption_gen.go` | Generate (new) |
| `internal/starlark/receiver_template_gen.go` | Generate (new) |
| `internal/starlark/receiver_package.go` | Delete |
| `internal/starlark/receiver_shell.go` | Delete |
| `internal/starlark/receiver_git.go` | Delete |
| `internal/starlark/receiver_http.go` | Delete |

## Phase 4: Wiring and Cleanup

### bindings.go

Update `Globals()` to construct generated receivers with Provider instances:

```go
"archive":  NewArchiveReceiver(output),
"service":  NewServiceReceiver(h.ServiceManager(), output),
"package":  NewPackageReceiver(h.PackageManager(), features, settings, output),
"shell":    NewShellReceiver(h, output),
"git":      NewGitReceiver(output),
"net":      NewNetReceiver(h, output),
// Docker and Npm stay hand-written
"docker":   NewDockerReceiver(output),
"npm":      NewNpmReceiver(output),
// Utilities stay hand-written
"env":      NewEnvReceiver(),
"log":      logRecv,
```

### platform/ directory

`internal/starlark/platform/` (darwin.go, linux.go, common.go) contains
platform-specific plan bindings that predate the Provider model. Providers handle
platform differences internally via `host.PackageManager` and `host.ServiceManager`.
These files become dead code after Phase 2. Delete them.

### Files

| File | Action |
|------|--------|
| `internal/starlark/bindings.go` | Modify: update receiver constructors |
| `internal/starlark/interfaces.go` | Modify: update PlanBindings interface |
| `internal/starlark/plan.go` | Modify: update planBindings implementation |
| `internal/starlark/platform/common.go` | Delete |
| `internal/starlark/platform/darwin.go` | Delete |
| `internal/starlark/platform/linux.go` | Delete |
| `internal/starlark/platform/windows.go` | Delete |

## Phase 5: Registry Script and Doc Updates

Starlark API changes affect lore manifest scripts in devlore-registry:

| Old API | New API |
|---------|---------|
| `plan.service("nginx", "start")` | `plan.service.start("nginx")` |
| `plan.shell("command")` | `plan.shell.exec("command")` |
| `plan.download(url)` | `plan.net.download(url)` |
| `plan.literal(content)` | `plan.content.literal(content)` |

Update all registry scripts. Update `docs/guides/lore/create-manifests.md`.
Regenerate knowledge extract outputs.

### Interaction with plan-to-phase rename

The separate `docs/plans/phase-binding.md` plan renames `plan.*` to `phase.*`
throughout the codebase. That plan runs AFTER this one. Since everything is
generated, the rename is a template variable change + regeneration.

## Verification

1. `go build ./...` — compiles
2. `go test ./internal/execution/...` — compensation tests pass
3. `go test ./internal/starlark/...` — receiver tests pass
4. Run `devlore ops.generate` for each provider, verify output matches committed `_gen.go`
5. Run a lore manifest dry-run to verify plan bindings create correct graph nodes
6. Run a star extension script to verify receivers call Provider methods
7. Regenerate knowledge extract, verify binding counts
