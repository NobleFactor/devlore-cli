---
title: "Phase 7: BindingConfig Receivers"
status: complete
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 7: BindingConfig Receivers

## Summary

Move receiver selection into `BindingConfig` so the program — not the
binding set — decides which receivers are available. Today receiver names
are hardcoded inside `prepareScriptEnv` (lore) and `Start` (test runner).
No caller in the chain gets to specify them.

After this phase, every program declares its receivers in config:

```go
cfg := op.BindingConfig{
    Writer:      os.Stdout,
    ProgramName: "lore",
    Color:       true,
    Receivers:   []string{"ui", "plan"},
}
```

`BindingSet.With()` is eliminated. `NewBindingSet(cfg)` reads
`cfg.Receivers` directly.

## Current State

Receiver names are hardcoded at the wrong level:

```
lore:  prepareScriptEnv()  → NewBindingSet(cfg).With("ui", "plan")     ← buried
test:  runner.Start()      → NewBindingSet(cfg).With("plan", "file")   ← buried
```

The programs that initiate graph construction (`lore`, `writ`, test runner)
have no way to specify which receivers they need. The decision is made
inside implementation details.

## Design

### Add `Receivers` to `BindingConfig`

```go
type BindingConfig struct {
    Writer      io.Writer
    ProgramName string
    Color       bool
    WorkDir     string
    Platform    *Platform
    Receivers   []string  // NEW — receiver namespaces to include as globals
}
```

### Update `NewBindingSet`

`NewBindingSet(cfg)` populates `included` from `cfg.Receivers`:

```go
func NewBindingSet(cfg op.BindingConfig) *BindingSet {
    included := make(map[string]bool, len(cfg.Receivers))
    for _, name := range cfg.Receivers {
        included[name] = true
    }
    return &BindingSet{
        cfg:      cfg,
        included: included,
        cache:    make(map[string]*loaderEntry),
    }
}
```

### Delete `With()`

No callers remain after all call sites set `cfg.Receivers`.

### Add `WithReceivers` to Runner

The test runner's `WithReceivers` option sets the receivers on the
`BindingConfig` it constructs internally:

```go
func WithReceivers(names ...string) Option {
    return func(r *Runner) { r.receivers = names }
}
```

In `Start()`:

```go
bs := loreStar.NewBindingSet(op.BindingConfig{
    Writer:      r.writer,
    ProgramName: "devlore-test",
    WorkDir:     tmpDir,
    Receivers:   r.receivers,
})
```

No default. Every test declares what it needs via `WithReceivers(...)`.

## Call Sites

| File | Before | After |
| --- | --- | --- |
| `pkg/op/binding_config.go` | no `Receivers` field | add `Receivers []string` |
| `internal/starlark/binding_set.go` | `NewBindingSet(cfg)` + `With()` | `NewBindingSet(cfg)` reads `cfg.Receivers`; delete `With()` |
| `internal/lore/builder.go` | `NewBindingSet(cfg).With("ui", "plan")` | `cfg.Receivers = []string{"ui", "plan"}` |
| `internal/starlark/integration_test.go` | `NewBindingSet(cfg).With("ui")` | `cfg.Receivers = []string{"ui"}` |
| `internal/starlark/binding_set_test.go` | `NewBindingSet(cfg).With(...)` | `cfg.Receivers = [...]` |
| `internal/e2e/testrunner/runner.go` | `NewBindingSet(cfg).With("plan", "file")` | `cfg.Receivers = r.receivers` |
| `internal/e2e/testrunner/runner_test.go` | `runScript(t, name)` | `runScript(t, name, WithReceivers("plan", "file"))` |

## Tasks

- [x] Add `Receivers []string` to `BindingConfig`
- [x] Update `NewBindingSet` to populate `included` from `cfg.Receivers`
- [x] Delete `With()` method from `BindingSet`
- [x] Add `WithReceivers(...string)` option to test `Runner`
- [x] Remove hardcoded `.With("plan", "file")` from `runner.Start()`
- [x] Update `lore/builder.go`: set `Receivers` in config
- [x] Update `integration_test.go`: set `Receivers` in config
- [x] Update `binding_set_test.go`: set `Receivers` in config
- [x] Update all existing e2e test functions to pass `WithReceivers` explicitly
- [x] Remove Phase 6 skip annotations from immediate tests
- [x] All e2e tests pass (`make test`)
- [x] `make test-race` passes

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/binding_config.go` | Modify | Add `Receivers` field |
| `internal/starlark/binding_set.go` | Modify | Read from config; delete `With()` |
| `internal/lore/builder.go` | Modify | Set `Receivers` in config |
| `internal/starlark/integration_test.go` | Modify | Set `Receivers` in config |
| `internal/starlark/binding_set_test.go` | Modify | Set `Receivers` in config |
| `internal/e2e/testrunner/runner.go` | Modify | Add `WithReceivers`; use `cfg.Receivers` |
| `internal/e2e/testrunner/runner_test.go` | Modify | Pass `WithReceivers` everywhere |

## Additional Work

### Makefile fixes (discovered during Phase 6/7)

Three bugs in the Makefile prevented correct incremental codegen:

1. **json, regexp, yaml under wrong section**: These providers changed from
   `access=immediate` to `access=both` but their Makefile rules still listed
   only immediate outputs. Moved to `access=both` section with full output list.

2. **`provider.gen.go` not tracked**: Missing from all 18 grouped targets.
   Any change to `provider.gen.go` didn't trigger dependent rebuilds.

3. **`actions.gen.go` phantom target**: Listed as an output in `access=both`
   rules but never generated. GNU Make's `&:` grouped target ran the recipe
   on every `make build` because the file was perpetually missing.

### register.go — missing blank imports

Seven providers were missing from `pkg/op/provider/register.go`:
`json`, `regexp`, `yaml`, `staranalysis`, `starcomplexity`, `starindex`,
`starstats`. Without blank imports, `init()` → `op.Announce()` never runs
and the providers are invisible to the binding set.

### Immediate test fixes

Several test scripts needed corrections after un-skipping:

| Script | Fix |
| --- | --- |
| `test_imm_file.star` | `type()` returns `"struct"`, not `"resource"` — Resource marshals as generic Starlark struct |
| `test_imm_json.star` | `encode` needs `value=` keyword; `encode_indent` needs `indent=` param |
| `test_imm_shell.star` | `shell.exec` returns command string, not stdout |
| `test_imm_template.star` | `content` param is `[]byte` — changed string literal to `b"..."` |
| `test_imm_ui.star` | Removed `ui.fail()` — terminates script by design |

### Dry-run tests for json, regexp, yaml

Added planned (dry-run) tests for providers that gained `access=both`:

- `test_json.star` — `plan.json.encode`, `encode_indent`, `decode` (3 nodes)
- `test_yaml.star` — `plan.yaml.encode`, `decode` (2 nodes)
- `test_regexp.star` — all 8 regexp planned actions

### devloretest CLI

`internal/devloretest/commands.go` was not passing receivers to the test
runner. Added `WithReceivers("plan", "file")` to the options list.

## Exit Criteria

- [x] `BindingConfig.Receivers` is the sole source of receiver selection
- [x] `BindingSet.With()` does not exist
- [x] No hardcoded receiver names in `runner.go` or `builder.go`
- [x] All Phase 6 e2e tests pass (no skips for missing receivers)
- [x] `make check` passes
- [x] `make test-race` passes
