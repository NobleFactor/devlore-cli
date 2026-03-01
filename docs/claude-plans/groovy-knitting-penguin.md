# Phase 3: Migrate Commands to Extensions (Worker 4)

## Summary

Create extension YAML specs for existing star commands, packaging the ops/*.star implementations into the extension directory structure.

## Scope

**In scope:**
- lint.* commands (go, shell, markdown, tools, all, copyright)
- config.* commands (show, sync)
- setup.* commands (root, tools, hooks, config, check)
- hook.* commands (pre-commit, pre-push)

**Deferred:**
- distill.* (needs design work)

**Not extensions:**
- self.* (built-in commands)

## Extension Directory Structure

ExtensionSpec supports ONE command per extension. Each subcommand needs its own extension.

```
extensions/
├── lint-go/
│   ├── extension.yaml
│   └── lint-go.star
├── lint-shell/
│   ├── extension.yaml
│   └── lint-shell.star
├── lint-markdown/
│   ├── extension.yaml
│   └── lint-markdown.star
├── lint-tools/
│   ├── extension.yaml
│   └── lint-tools.star
├── lint-all/
│   ├── extension.yaml
│   └── lint-all.star
├── lint-copyright/
│   ├── extension.yaml
│   └── lint-copyright.star
├── config-show/
│   ├── extension.yaml
│   └── config-show.star
├── config-sync/
│   ├── extension.yaml
│   └── config-sync.star
├── setup/
│   ├── extension.yaml
│   └── setup.star
├── setup-tools/
│   ├── extension.yaml
│   └── setup-tools.star
├── setup-hooks/
│   ├── extension.yaml
│   └── setup-hooks.star
├── setup-config/
│   ├── extension.yaml
│   └── setup-config.star
├── setup-check/
│   ├── extension.yaml
│   └── setup-check.star
├── hook-pre-commit/
│   ├── extension.yaml
│   └── hook-pre-commit.star
└── hook-pre-push/
    ├── extension.yaml
    └── hook-pre-push.star
```

**Total: 15 extensions**

## Files to Create

### 1. extensions/lint-go/extension.yaml
```yaml
extension: lint.go
description: Run Go lint checks (go mod tidy + golangci-lint)

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      go: Run golangci-lint with mod tidy check
      ensure_tools: Check lint tool availability

command:
  help: Run Go lint checks (go mod tidy + golangci-lint)
  implementation: lint-go.star

flags:
  - name: path
    type: string
    default: "./..."
    help: Path to lint
  - name: config
    type: string
    default: ""
    help: Path to golangci-lint config file
  - name: skip_mod_tidy
    type: bool
    default: "false"
    help: Skip go mod tidy check
```

### 2. extensions/lint-shell/extension.yaml
```yaml
extension: lint.shell
description: Run shellcheck and shfmt on shell scripts

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      shell: Run shellcheck and shfmt

command:
  help: Run shellcheck and shfmt on shell scripts
  implementation: lint-shell.star

flags:
  - name: path
    type: string
    default: "."
    help: Path to lint
  - name: severity
    type: string
    default: "warning"
    help: Minimum shellcheck severity (error, warning, info, style)
  - name: indent
    type: int
    default: "4"
    help: Expected indent size for shfmt
```

### 3. extensions/lint-markdown/extension.yaml
```yaml
extension: lint.markdown
description: Run markdownlint and frontmatter check

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      markdown: Run markdownlint with frontmatter validation

command:
  help: Run markdownlint and frontmatter check on markdown files
  implementation: lint-markdown.star

flags:
  - name: path
    type: string
    default: "."
    help: Path to lint
  - name: fix
    type: bool
    default: "false"
    help: Auto-fix issues where possible
```

### 4. extensions/lint-tools/extension.yaml
```yaml
extension: lint.tools
description: Check status of required lint tools

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      ensure_tools: Check lint tool availability

command:
  help: Check status of required lint tools
  implementation: lint-tools.star

flags: []
```

### 5. extensions/lint-all/extension.yaml
```yaml
extension: lint.all
description: Run all configured linters

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      go: Run Go linter
      shell: Run shell linter
      markdown: Run markdown linter
  - name: copyright
    builtin: true
    type: CopyrightChecker
    functions:
      check: Verify copyright headers
      fix: Add or update headers
  - name: config
    builtin: true
    type: ConfigReceiver
    functions:
      get: Load merged config

command:
  help: Run all configured linters
  implementation: lint-all.star

flags:
  - name: fix
    type: bool
    default: "false"
    help: Auto-fix issues where possible
```

### 6. extensions/lint-copyright/extension.yaml
```yaml
extension: lint.copyright
description: Check or fix copyright headers in source files

receivers:
  - name: copyright
    builtin: true
    type: CopyrightChecker
    functions:
      check: Verify files have correct headers
      fix: Add or update headers
      detect_license: Detect SPDX from LICENSE file
  - name: config
    builtin: true
    type: ConfigReceiver
    functions:
      get: Load merged config
  - name: fs
    builtin: true
    type: FSReceiver
    functions:
      glob: Find files matching pattern

command:
  help: Check or fix copyright headers in source files
  implementation: lint-copyright.star

flags:
  - name: fix
    type: bool
    default: "false"
    help: Add missing headers and update old format
  - name: path
    type: string
    default: "."
    help: Path to check

config:
  type: CopyrightConfig
  fields:
    enabled: bool
    license: string
    holder: string
    patterns: map[string]any
    exclude: "[]string"
  defaults:
    enabled: false
    license: "auto"
```

### 7. extensions/config-show/extension.yaml
```yaml
extension: config.show
description: Show merged configuration from star.yaml hierarchy

receivers:
  - name: config
    builtin: true
    type: ConfigReceiver
    functions:
      show: Show config with sources

command:
  help: Show merged configuration from star.yaml hierarchy
  implementation: config-show.star

flags: []
```

### 8. extensions/config-sync/extension.yaml
```yaml
extension: config.sync
description: Sync tool configs from star.yaml

receivers:
  - name: config
    builtin: true
    type: ConfigReceiver
    functions:
      sync: Write tool-specific config files

command:
  help: Sync tool configs (.golangci.yaml, etc.) from star.yaml
  implementation: config-sync.star

flags: []
```

### 9. extensions/setup/extension.yaml
```yaml
extension: setup
description: Run all setup tasks

receivers:
  - name: setup
    builtin: true
    type: SetupReceiver
    functions:
      tools: Check tool installation status
      init_config: Initialize star.yaml
      install_hook: Install git hooks

command:
  help: Run all setup tasks (tools check, config init, hooks install)
  implementation: setup.star

flags: []
```

### 10. extensions/setup-tools/extension.yaml
```yaml
extension: setup.tools
description: Show required development tools and installation status

receivers:
  - name: setup
    builtin: true
    type: SetupReceiver
    functions:
      tools: Check tool installation status

command:
  help: Show required development tools and installation status
  implementation: setup-tools.star

flags: []
```

### 11. extensions/setup-hooks/extension.yaml
```yaml
extension: setup.hooks
description: Install pre-commit hooks

receivers:
  - name: setup
    builtin: true
    type: SetupReceiver
    functions:
      install_hook: Install git hooks

command:
  help: Install pre-commit hooks
  implementation: setup-hooks.star

flags: []
```

### 12. extensions/setup-config/extension.yaml
```yaml
extension: setup.config
description: Initialize star.yaml and sync tool configurations

receivers:
  - name: setup
    builtin: true
    type: SetupReceiver
    functions:
      init_config: Initialize star.yaml

command:
  help: Initialize star.yaml and sync tool configurations
  implementation: setup-config.star

flags: []
```

### 13. extensions/setup-check/extension.yaml
```yaml
extension: setup.check
description: Check setup status without making changes

receivers:
  - name: setup
    builtin: true
    type: SetupReceiver
    functions:
      tools: Check tool installation status
      check_hook: Check hook installation status

command:
  help: Check setup status without making changes
  implementation: setup-check.star

flags: []
```

### 14. extensions/hook-pre-commit/extension.yaml
```yaml
extension: hook.pre-commit
description: Run pre-commit checks (called by git pre-commit hook)

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      go: Run Go linter
      shell: Run shell linter
      markdown: Run markdown linter
      ensure_tools: Check tool availability

command:
  help: Run pre-commit checks (called by git pre-commit hook)
  implementation: hook-pre-commit.star

flags: []
```

### 15. extensions/hook-pre-push/extension.yaml
```yaml
extension: hook.pre-push
description: Run pre-push checks (called by git pre-push hook)

receivers:
  - name: lint
    builtin: true
    type: LintReceiver
    functions:
      go: Run Go linter
      shell: Run shell linter
      markdown: Run markdown linter

command:
  help: Run pre-push checks (called by git pre-push hook)
  implementation: hook-pre-push.star

flags: []
```

## Starlark Implementations

Extract from ops/*.star into individual extension .star files:

| Source | Target | Functions to extract |
|--------|--------|---------------------|
| ops/lint.star | extensions/lint-go/lint-go.star | run_go, check_tool, ensure_tool_installed |
| ops/lint.star | extensions/lint-shell/lint-shell.star | run_shell |
| ops/lint.star | extensions/lint-markdown/lint-markdown.star | run_markdown |
| ops/lint.star | extensions/lint-tools/lint-tools.star | run_tools |
| ops/lint.star | extensions/lint-all/lint-all.star | run_all, run_*_silent helpers |
| ops/lint-copyright.star | extensions/lint-copyright/lint-copyright.star | run_copyright, collect_source_files |
| ops/config.star | extensions/config-show/config-show.star | run_show, _print_config |
| ops/config.star | extensions/config-sync/config-sync.star | run_sync |
| ops/setup.star | extensions/setup/setup.star | run_setup |
| ops/setup.star | extensions/setup-tools/setup-tools.star | run_tools |
| ops/setup.star | extensions/setup-hooks/setup-hooks.star | run_hooks |
| ops/setup.star | extensions/setup-config/setup-config.star | run_config |
| ops/setup.star | extensions/setup-check/setup-check.star | run_check |
| ops/hook.star | extensions/hook-pre-commit/hook-pre-commit.star | run_pre_commit, run_linter, run_*_check |
| ops/hook.star | extensions/hook-pre-push/hook-pre-push.star | run_pre_push |

## Notes

- **Shared helpers**: lint-all and hook-pre-commit duplicate lint check logic. Consider extracting to a shared module in future refactoring.
- **Legacy ops/ cleanup**: Keep ops/*.star during Phase 3 transition. Remove in Phase 5 cleanup after runtime integration confirmed.

## Verification

```bash
# Parse all extension specs
go test ./internal/extension/...

# Build
go build ./...

# Test commands (after Worker 2 integrates runtime)
./star lint go
./star lint shell
./star lint copyright
./star setup config
./star hook pre-commit
```

## Dependencies

- Phase 2 complete: `internal/extension/spec.go` ✓
- Worker 2 Phase 3: runtime.go loads extensions from extensions/ directory
