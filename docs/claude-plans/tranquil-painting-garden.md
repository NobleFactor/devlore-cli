# Code-Level Terminology Rename: Human-Facing File Names

## Context

Doc-level terminology is already updated (previous session work). The code still
uses the old names:

- Template dict keys: `"plan_receiver"`, `"realtime_receiver"`
- Template filenames: `plan_receiver.go.template`, `realtime_receiver.go.template`
- Generated output: `plan_%s_gen.go`, `receiver_%s_gen.go`

The user said: "I want all files named using the human-facing terminology."

Target terminology:
- "planned receiver" (was "plan receiver")
- "immediate receiver" (was "realtime receiver")

---

## Step 1: Rename template files on disk

User executes (cannot delete files):

```bash
cd star/extensions/com.noblefactor.devlore.Actions/templates
mv plan_receiver.go.template planned_receiver.go.template
mv realtime_receiver.go.template immediate_receiver.go.template
```

`graph_actions.go.template` — unchanged.

---

## Step 2: Update `generate.star`

**File**: `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star`

### 2a: Header comment (line 8)

```
# realtime receivers.
```
→
```
# immediate receivers.
```

### 2b: Three dict literals (lines 18-36)

```starlark
TEMPLATE_FILES = {
    "planned_receiver": "planned_%s_gen.go",
    "graph_actions": "actions_gen.go",
    "immediate_receiver": "immediate_%s_gen.go",
}

TEMPLATE_PACKAGES = {
    "planned_receiver": "starlark",
    "graph_actions": "",
    "immediate_receiver": "starlark",
}

LOCAL_TEMPLATES = {
    "planned_receiver": "planned_receiver.go.template",
    "graph_actions": "graph_actions.go.template",
    "immediate_receiver": "immediate_receiver.go.template",
}
```

### 2c: Auto-detect logic (lines 162-167)

```starlark
        if is_plannable:
            templates = ["planned_receiver", "graph_actions"]
            note("Detected //devlore:plannable — generating planned_receiver + graph_actions")
        else:
            templates = ["immediate_receiver"]
            note("No //devlore:plannable directive — generating immediate_receiver")
```

### 2d: Error message (line 184)

```starlark
            fail("unknown template: " + tmpl + " (valid: planned_receiver, graph_actions, immediate_receiver)")
```

### 2e: Namespace derivation (line 187)

```starlark
        if tmpl == "planned_receiver":
```

### 2f: Package derivation — no change needed

`tmpl == "graph_actions"` check on line 193 is unchanged.
`TEMPLATE_PACKAGES[tmpl]` on line 197 uses the new key automatically.

### 2g: Extra attrs guard (line 208)

```starlark
        if tmpl == "immediate_receiver" and extra_attrs:
```

### 2h: Comment on line 175

```starlark
    # Extra attrs from companion query files (for immediate_receiver)
```

---

## Step 3: Update source comments (3 files)

**`internal/starlark/output.go` line 10:**
```
// FillSlot calls in planned_*_gen.go files via the planFillSlots template
```

**`internal/starlark/plan_registry.go` line 20:**
```
// functions in generated planned_*_gen.go files.
```

**`internal/starlark/plan_root.go` line 19:**
```
// planned_*_gen.go registers via init()).
```

---

## Step 4: Delete old generated files, rebuild `star`, regenerate

User executes deletions:

```bash
# Delete old generated planned receivers
rm internal/starlark/plan_archive_gen.go
rm internal/starlark/plan_encryption_gen.go
rm internal/starlark/plan_file_gen.go
rm internal/starlark/plan_git_gen.go
rm internal/starlark/plan_net_gen.go
rm internal/starlark/plan_pkg_gen.go
rm internal/starlark/plan_service_gen.go
rm internal/starlark/plan_shell_gen.go
rm internal/starlark/plan_template_gen.go

# Delete old generated immediate receiver
rm internal/starlark/receiver_ui_gen.go
```

Rebuild `star` binary (picks up generate.star changes):

```bash
cd ../noblefactor-ops && go build -o ../devlore-cli.binding-unification/star/extensions/com.noblefactor.devlore.Actions/star ./cmd/star
```

Regenerate all providers (9 plannable + 1 immediate):

```bash
# Plannable providers (each produces planned_*_gen.go + actions_gen.go)
star devlore.actions.generate --source=internal/execution/provider/file --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/pkg --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/shell --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/service --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/net --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/archive --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/git --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/content --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/template --output=internal/starlark --write=true
star devlore.actions.generate --source=internal/execution/provider/encryption --output=internal/starlark --write=true

# Immediate-only provider (produces immediate_ui_gen.go)
star devlore.actions.generate --source=internal/execution/provider/ui --output=internal/starlark --write=true
```

New files produced:
- `planned_archive_gen.go`, `planned_encryption_gen.go`, `planned_file_gen.go`, `planned_git_gen.go`, `planned_net_gen.go`, `planned_pkg_gen.go`, `planned_service_gen.go`, `planned_shell_gen.go`, `planned_template_gen.go`
- `immediate_ui_gen.go`
- 10 `actions_gen.go` files (overwritten in place, names unchanged)

---

## Step 5: Update doc references

All `plan_*_gen.go` → `planned_*_gen.go` and `receiver_*_gen.go` → `immediate_*_gen.go`
in docs. Also update template name references (`plan_receiver` → `planned_receiver`,
`realtime_receiver` → `immediate_receiver`) that appear in code examples.

| File | Lines | Change |
|------|-------|--------|
| `docs/architecture/projected-provider-api.md` | 45, 46, 123, 207-208, 283, 375 | Template names + output patterns |
| `docs/plans/projected-provider-api.md` | 72-73 | `plan_*_gen.go` → `planned_*_gen.go`, `receiver_*_gen.go` → `immediate_*_gen.go` |
| `docs/plans/resource-provider.md` | 59, 224, 232 | Template names |
| `docs/plans/resource-provider/phase-3.md` | 1, 19, 48, 154, 259, 261, 306, 319 | Template names + output patterns |
| `docs/plans/resource-provider/phase-2b.md` | 212 | Template name |
| `docs/plans/binding-unification.md` | 15, 54, 96 | Template names |
| `docs/plans/binding-unification/phase-1.md` | 20 | Template filename |
| `docs/plans/binding-unification/phase-8.md` | 64, 106 | Template names + output patterns |
| `docs/plans/binding-unification/phase-9.md` | 118-119, 167-168, 178, 194 | Template names + output patterns |
| `docs/plans/binding-unification/part-23.md` | 79 | `plan_*_gen.go` → `planned_*_gen.go` |
| `docs/plans/star-gen-receiver.md` | 271, 350, 352 | Template names |
| `docs/plans/star-gen-receiver/phase-2.md` | 118, 319, 358, 374 | Template names |
| `docs/plans/star-gen-receiver/phase-3.md` | 39, 41, 46, 48, 53, 55, 119, 154, 156, 161, 163, 259, 267, 270, 272 | Template names + output patterns |
| `docs/plans/star-gen-receiver/phase-4.md` | 228, 250 | Template names |
| `docs/plans/star-gen-receiver/phase-5.md` | 139, 298, 359, 372, 627-628 | Template names + rm commands |
| `docs/plans/star-gen-receiver/phase-6.md` | 650, 656, 773-774 | Template names + rm commands |

---

## Verification

```bash
# All old filenames gone
ls internal/starlark/plan_*_gen.go 2>&1  # should error
ls internal/starlark/receiver_*_gen.go 2>&1  # should error

# New filenames exist
ls internal/starlark/planned_*_gen.go  # 9 files
ls internal/starlark/immediate_*_gen.go  # 1 file (ui)

# No old template names in generate.star
grep -c "plan_receiver\|realtime_receiver" star/extensions/com.noblefactor.devlore.Actions/commands/generate.star
# Expected: 0

# No old output patterns in source comments
grep -rn "plan_\*_gen\|receiver_\*_gen" internal/
# Expected: 0

# Build and test
make build
make test
```
