# Deployment Receipts

Every time `writ deploy` runs, it creates a **receipt** recording exactly what was deployed. Receipts are essential for:

- **Auditing**: See what files were deployed and when
- **Rollback**: Understand what to undo if something goes wrong
- **Verification**: Confirm deployments match expectations
- **Sharing**: Coordinate with team members on environment changes

## Receipt Location

Receipts are stored in the unified devlore state directory:

```
~/.local/state/devlore/receipts/
```

Each tool creates receipts with its name prefix:

```
writ-2025-01-29T10-30-00.yaml    # writ deployment
lore-2025-01-29T11-00-00.yaml    # lore package installation
writ-latest.yaml                  # symlink to most recent writ receipt
lore-latest.yaml                  # symlink to most recent lore receipt
```

## Viewing Receipts

### Show the Latest Receipt

```bash
writ receipt show
```

### List All Receipts

```bash
writ receipt list
```

### Show a Specific Receipt

```bash
writ receipt show writ-2025-01-29T10-30-00.yaml
```

## Receipt Contents

A v4 receipt contains:

```yaml
version: "4"
format: graph
timestamp: 2025-01-29T10:30:00Z
tool: writ
platform:
  os: darwin
  arch: arm64
context:
  source_root: /Users/me/.local/share/devlore/repos
  target_root: /Users/me
  projects: [base, team, personal]
  segments: {os: darwin, arch: arm64}
roots: [base, team, personal]
nodes:
  - id: .config/git/config
    operation: link
    status: completed
    timestamp: "2025-01-29T10:30:00Z"
    source: /Users/me/.local/share/devlore/repos/base/.config/git/config
    target: /Users/me/.config/git/config
    project: base
    layer: base
summary:
  total_files: 42
  links: 38
  templates: 3
  secrets: 1
  skipped: 0
checksum: "sha256:a7b9c3d4e5f6..."
```

### Fields

| Field | Description |
|-------|-------------|
| `version` | Receipt format version (current: "4") |
| `format` | Always "graph" for v4 receipts |
| `timestamp` | When the deployment completed |
| `tool` | Which tool created the receipt (`writ` or `lore`) |
| `platform` | OS and architecture where deployment ran |
| `context` | Source/target paths, projects, segments |
| `nodes` | Individual file operations |
| `summary` | Aggregated statistics |
| `checksum` | Integrity hash of receipt contents |
| `signature` | Optional cryptographic signature |

## Integrity Verification

### Checksum

Every receipt includes a checksum computed using a git-style algorithm:

```
SHA256("receipt <filename> <length>\0<content>")
```

This detects any modification to the receipt contents, whether accidental or intentional.

### Verifying Integrity

```bash
writ receipt verify
```

This checks:
1. Checksum matches receipt contents
2. Signature is valid (if present)

### Signature (Optional)

Receipts can include a cryptographic signature for authenticity. This is useful when sharing receipts with team members.

To sign receipts, configure an age identity:

```bash
# Set age key path
export SOPS_AGE_KEY_FILE=~/.config/sops/age/keys.txt

# Deploy with signing
writ deploy --sign
```

## Using Receipts

### Check What Changed

Compare the latest receipt with what's currently deployed:

```bash
writ status
```

### Verify Deployed Files

Confirm files on disk match the receipt:

```bash
writ verify
```

### Find When Something Changed

Search receipts for a specific file:

```bash
writ receipt list --filter target=~/.config/git/config
```

## Receipt Versions

| Version | Description |
|---------|-------------|
| v1 | Initial format (flat entries) |
| v2 | Added source/target checksums |
| v3 | Added age-encrypted signature |
| v4 | Graph format with git-style checksum |

Legacy receipts (v1-v3) are automatically converted when loaded.

## Troubleshooting

### "checksum mismatch"

The receipt was modified after creation. This could indicate:
- Manual editing of the receipt file
- File corruption
- Tampering

Run `writ deploy` to regenerate a valid receipt.

### "signature invalid"

The signature cannot be verified. Possible causes:
- Receipt was modified after signing
- Wrong age identity used for verification
- Signature was created with a different key

### Receipts Not Found

Check the receipts directory exists:

```bash
ls -la ~/.local/state/devlore/receipts/
```

If empty, run `writ deploy` to create the first receipt.
