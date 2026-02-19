# Phase 2: Generate Plan Bindings for All Providers

**Status**: COMPLETE — PR #151

## Summary

Replace 4 hand-written plan files with generated ones. Move service, shell,
net, and content from top-level PlanRoot builtins to sub-namespaces.

## API Changes

| Old API | New API |
|---------|---------|
| `plan.service("nginx", "start")` | `plan.service.start("nginx")` |
| `plan.shell("command")` | `plan.shell.exec("command")` |
| `plan.download(url)` | `plan.net.download(url)` |
| `plan.literal(content)` | `plan.content.literal(content)` |

PlanRoot keeps only `plan.source(path)` and `plan.gather(...)` as top-level
builtins (graph construction primitives, not resource operations).

## Files

| File | Action |
|------|--------|
| `plan_file_gen.go` | Generate (replaces plan_file.go) |
| `plan_package_gen.go` | Generate (replaces plan_package.go) |
| `plan_encryption_gen.go` | Generate (replaces plan_encryption.go) |
| `plan_template_gen.go` | Generate (replaces plan_template.go) |
| `plan_service_gen.go` | Generate (new) |
| `plan_shell_gen.go` | Generate (new) |
| `plan_net_gen.go` | Generate (new) |
| `plan_content_gen.go` | Generate (new) |
| `plan_file.go` | Delete |
| `plan_package.go` | Delete |
| `plan_encryption.go` | Delete |
| `plan_template.go` | Delete |
| `plan_root.go` | Modify: add sub-namespaces, remove builtins |

## Design Decisions

- Variadic args for PackagePlan: `[]string` params become variadic positional args
- noblefactor-ops `planUnpackArgs` template helper updated to handle this
