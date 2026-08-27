---
title: "How to create and modify providers"
description: "How to create and modify providers in devlore-cli"
tool: "devlore"
category: "tutorial"
order: 10
---

# How to create and modify providers

Providers live in `pkg/op/provider/<name>/`. Each provider has a `Provider` struct.
Binding level derives from method classification — see
[3.6-method-classification.md](../architecture/3.6-method-classification.md).

Dependent types (non-primitive return types) produce additional
`gen/<type_snake>.gen.go` files automatically.

## Development loop

1. Edit `provider.go` — add or modify methods on the `Provider` struct
2. Ensure the Makefile grouped target lists every gen file for the provider's access level
3. Update tests
4. `make test` — triggers `generate` which regenerates stale gen files, then runs all tests
5. Debug failures and repeat

## Makefile rules

The Makefile uses GNU Make grouped targets (`&:`) so one `star` invocation
produces all gen files for a provider. Generation fires only when `provider.go`
is newer than the gen outputs.

```makefile
# access=both example
$(P)/file/gen/actions.gen.go \
$(P)/file/gen/immediate.gen.go \
$(P)/file/gen/params.gen.go \
$(P)/file/gen/planned.gen.go &: $(P)/file/provider.go | star

# access=immediate example
$(P)/json/gen/immediate.gen.go \
$(P)/json/gen/params.gen.go &: $(P)/json/provider.go | star
```

Every provider must appear in the `generate` target's dependency list.

## Provider struct directives

```go
// +devlore:lifetime=stateless
// +devlore:starlarkbridge Root=WorkDir
type Provider struct {
    Root string
}
```

| Directive | Values | Default |
|---|---|---|
| `+devlore:lifetime` | `stateless`, `phase`, `session` | `stateless` |
| `+devlore:bind` | `Field=CfgField` | none |
| `+devlore:surface` | `graph`, `module` | both |
| `+devlore:root` | `true` | `false` |

### How `surface` and `root` become roles

`+devlore:surface=` decides which **dispatch** zones a provider holds; `+devlore:root=true` sets the
**placement** bit. The two are orthogonal, and the generator composes them:

| `+devlore:surface=` | Roles | with `+devlore:root=true` |
|---|---|---|
| *absent* — the default | `RoleModule\|RoleAction` | `RoleModule\|RoleAction\|RoleRoot` |
| `graph` | `RoleAction` | `RoleAction\|RoleRoot` |
| `module` | `RoleModule` | `RoleModule\|RoleRoot` |

Absent is the default because a graph accepts anything with an action signature, and module membership is
decided per **method** per runtime from its claims — never per provider. Only two providers declare a surface,
and they are opposites: `flow` is `graph` (its methods *are* the graph combinators and mean nothing outside
one) and `plan` is `module` (there being no scenario for planning the planner).

**`root` applies to every surface the provider reaches.** One bit, both namespaces. A provider with the
default surface and `root=true` surfaces flat in both: `ui` gives a script `note(...)` and a graph
`plan.note(...)` from a single directive. That is intended — a name is promoted because it reads better
without its qualifier, and that is as true of a graph as of a script.

## Method directives

```go
// +devlore:defaults gitignore=true
// +devlore:struct_param cfg=AnalysisConfig
func (p *Provider) Capture(pattern string, gitignore bool) (*Sources, error) {
```

| Directive | Purpose |
|---|---|
| `+devlore:defaults` | Mark params as optional with default values |
| `+devlore:struct_param` | Expand a struct param to individual Starlark kwargs |

## Adding a new provider

1. Create `pkg/op/provider/<name>/provider.go` with a `Provider` struct
2. Add methods — the generator discovers them automatically
3. Add a grouped target to the Makefile
4. Add the provider to the `generate` target's dependency list
5. Run `make test`
