# Issue #65 Error Scan Results

## Summary

Comprehensive scan for errors of judgment as documented in issue #65.

**Status: 1 CRITICAL ISSUE FOUND**

---

## Critical Issue

### `runAdoptFromReceipt()` Returns Success Without Implementation

**File:** `internal/writ/commands.go:1202-1211`

```go
func runAdoptFromReceipt(receiptPath, layer, project string, verbose, dryRun bool) error {
    // TODO: Implement reading lore receipt and adopting packages-manifest.yaml + config
    if receiptPath == "" {
        cli.Warn("adopt --from-receipt: not yet implemented (would use most recent receipt)")
    } else {
        cli.Warn("adopt --from-receipt %s: not yet implemented", receiptPath)
    }
    return nil  // <-- BUG: Returns success while admitting it's unimplemented
}
```

**Why this is critical:**
- This is the EXACT pattern from issue #65: "Left GCP KMS verify as stub returning success"
- Callers expect `nil` to mean the operation succeeded
- The function name and parameters suggest it performs work, but it does nothing
- Only prints a warning, then lies about success

---

## Plan

### Step 1: Fix the stub function
**File:** `internal/writ/commands.go:1210`

Change:
```go
return nil
```
To:
```go
return fmt.Errorf("adopt --from-receipt: not yet implemented")
```

### Step 2: Create feature issue for implementation

Create GitHub issue with the following template:

---

**Title:** `Implement writ adopt --from-receipt`

**Body:**
```markdown
## Summary

Implement the `--from-receipt` flag for `writ adopt` that reads a lore receipt and adopts the `packages-manifest.yaml` and config files into the environment repository.

## Current State

The function `runAdoptFromReceipt()` is a stub that returns an error (previously returned success, fixed in #65).

**Location:** `internal/writ/commands.go:1202-1211`

## Requirements

1. Read a lore receipt from the specified path (or `~/.local/state/devlore/receipts/lore-latest.yaml` if not specified)
2. Parse the receipt to extract:
   - `packages-manifest.yaml` that was deployed
   - Any config files that were templated
3. Adopt these files into the specified layer/project using the existing adopt logic

## Documentation References

- [writ adopt CLI docs](docs/cli/writ/adopt.md) - Documents the `--from-receipt` flag
- [Receipts User Guide](docs/guides/writ/receipts.md) - Receipt format and location
- [Receipt Integrity Architecture](docs/architecture/receipt-integrity.md) - Receipt structure (v4 format)

## Acceptance Criteria

- [ ] `writ adopt --from-receipt` reads the most recent lore receipt
- [ ] `writ adopt --from-receipt <path>` reads the specified receipt
- [ ] Adopted files are placed in `<layer>/<scope>/<project>/`
- [ ] Symlinks are created back to original locations
- [ ] Tests verify the feature works correctly
```

---

## Remediated Issues (Verified Clean)

| Pattern | Status | Evidence |
|---------|--------|----------|
| Decryptor `func([]byte)` fallback | CLEAN | `execution_test.go:244-257` rejects legacy signature |
| `packages.manifest` filename | CLEAN | `tree_test.go:235-280` explicitly rejects legacy filename |
| `LoreSchema`/`WritSchema` aliases | CLEAN | Only `DevloreSchema` exists in `schema/schema.go` |
| Unused `w io.Writer` parameter | CLEAN | All io.Writer params are used |
| `SourceRoot` compat hack | CLEAN | Legitimate usage, no compat code |
| Platform stubs | CLEAN | All properly `panic()` if called |

---

## Files to Modify

- `internal/writ/commands.go:1210` - Change `return nil` to `return fmt.Errorf(...)`
