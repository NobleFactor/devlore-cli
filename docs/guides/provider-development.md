---
title: "Provider Development"
description: "How to create and modify providers in devlore-cli"
tool: "devlore"
category: "development"
order: 10
---

# Provider Development

Providers live in `pkg/op/provider/<name>/`. Each provider has a `Provider` struct
annotated with `+devlore:access` to declare its binding level.

## Access levels

| Annotation | Gen files | Description |
|---|---|---|
| `+devlore:access=immediate` | immediate + params | Starlark builtins only |
| `+devlore:access=both` | actions + immediate + params + planned | Builtins + graph nodes |

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
// +devlore:access=both
// +devlore:lifetime=stateless
// +devlore:starlarkbridge Root=WorkDir
type Provider struct {
    Root string
}
```

| Directive | Values | Default |
|---|---|---|
| `+devlore:access` | `immediate`, `planned`, `both` | `immediate` |
| `+devlore:lifetime` | `stateless`, `phase`, `session` | `stateless` |
| `+devlore:bind` | `Field=CfgField` | none |

## Method directives

```go
// +devlore:defaults gitignore=true,includeBzl=true
// +devlore:struct_param cfg=AnalysisConfig
func (p *Provider) Capture(pattern string, gitignore, includeBzl bool) (*Sources, error) {
```

| Directive | Purpose |
|---|---|
| `+devlore:defaults` | Mark params as optional with default values |
| `+devlore:struct_param` | Expand a struct param to individual Starlark kwargs |

## Adding a new provider

1. Create `pkg/op/provider/<name>/provider.go` with a `Provider` struct
2. Annotate the struct with `+devlore:access=<level>`
3. Add methods — the generator discovers them automatically
4. Add a grouped target to the Makefile matching the access level
5. Add the provider to the `generate` target's dependency list
6. Run `make test`
