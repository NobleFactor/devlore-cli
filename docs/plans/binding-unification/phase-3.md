# Phase 3: Generate Receivers for All Providers

**Status**: COMPLETE — PR #151

## Summary

Generate receivers that call Provider methods for all 10 providers. Hand-written
query/convenience methods move to companion `_queries.go` files.

## Query Methods (hand-written, not Provider operations)

| Receiver | Query methods |
|----------|--------------|
| package | manager(), installed(name), version(name), feature(name), setting(name) |
| git | installed(), version(), repo_root(), current_branch(), remote_url(), is_clean(), latest_tag(), commit_hash() + 14 kwargs pass-through commands |
| shell | which(name) |
| net | get(url) |

## Files

| File | Action |
|------|--------|
| `receiver_file_gen.go` | Generate (new) |
| `receiver_package_gen.go` | Generate (replaces receiver_package.go) |
| `receiver_package_queries.go` | Create: hand-written queries |
| `receiver_service_gen.go` | Regenerate (now calls Provider) |
| `receiver_shell_gen.go` | Generate (replaces receiver_shell.go) |
| `receiver_shell_queries.go` | Create: hand-written (which) |
| `receiver_git_gen.go` | Generate (replaces receiver_git.go) |
| `receiver_git_queries.go` | Create: hand-written (27 methods) |
| `receiver_archive_gen.go` | Regenerate (now calls Provider) |
| `receiver_net_gen.go` | Generate (replaces receiver_http.go) |
| `receiver_net_queries.go` | Create: hand-written (get) |
| `receiver_encryption_gen.go` | Generate (new) |
| `receiver_template_gen.go` | Generate (new) |
| `receiver_package.go` | Delete |
| `receiver_shell.go` | Delete |
| `receiver_git.go` | Delete |
| `receiver_http.go` | Delete |

## Design Decisions

- Generator accepts `--extra-attrs` flag for companion file attributes
- Docker (21 methods) and Npm (17 methods) remain hand-written — no Provider
