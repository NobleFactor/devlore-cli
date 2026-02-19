# Phase 6: New Lifecycle Script Programming Model

**Status**: Planning

## Programming Model

Three concerns cleanly separated:

- **`plan`** — global, verb, graph construction
- **`phase`** — argument, noun, phase context (name, action, retry)
- **`package`** — argument, data (name, version, features, settings)

Entry point named for the lifecycle phase:

```python
def install(package, phase):
    note("installing %s %s" % (package.name, package.version))
    phase.retry(max_attempts=3, backoff="exponential")
    plan.package.install("docker-ce", "docker-ce-cli", "containerd.io")
    plan.service.enable("docker")
    plan.service.start("docker")
```

## Entry Point Changes

| Old | New |
|-----|-----|
| `def forward(package, system, plan):` | `def install(package, phase):` |
| `def compensate(package, system, plan):` | Deleted — Action Do/Undo handles compensation |
| `def configure(phase):` | Deleted — absorbed into `phase.retry()` |

## Key Decisions

- `plan` stays `plan` (NOT renamed to `phase`) — it is the verb
- `phase` is the noun — phase context passed as argument
- Output functions (`note`, `warn`, `error`, `success`, `fail`) are globals
- Choose predicates replace system probes (execute at runtime, not plan time)
- Retry is node-attachable, not just phase-level
- Phase-binding plan (`docs/plans/phase-binding.md`) fully superseded

## Changes

1. New entry point and script environment (builder.go)
2. Delete 6 system binding files + phase_config.go
3. Modify builder: phase-named entry point, `plan` global, 2 call args
4. Add execution-time predicates for Choose nodes
5. Node-attachable retry (RetryPolicy as Starlark object)
6. Update all registry scripts (devlore-registry)
7. Update knowledge extract pipeline
