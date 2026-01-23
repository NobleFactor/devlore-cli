---
title: "Secrets Management"
description: "Encrypt sensitive files with age for safe storage in git"
tool: "writ"
category: "tutorial"
order: 4
---

# Secrets Management

Writ uses [age encryption](https://age-encryption.org/) to store sensitive files
(API tokens, credentials, private keys) in your repository. Files are encrypted
at rest and decrypted during deployment.

## How it works

Files ending in `.age` are treated as encrypted secrets. During `writ add`,
they are decrypted using your identity (SSH key or age key) and the plaintext
is copied to the target location.

```
noblefactor/
└── .config/
    ├── github/token.age        # Encrypted in git
    └── ssh/config              # Normal symlink
```

After deployment:

```
~/.config/github/token          # Decrypted copy (not a symlink)
~/.config/ssh/config            # Symlink to source
```

## Setup

### Recipients file

Create a `.age-recipients` file in your repository root listing the public keys
of machines that should be able to decrypt secrets:

```
# .age-recipients
# Work laptop (SSH key)
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... work-laptop

# Home desktop (age key)
age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p
```

### Identity resolution

Writ resolves your decryption identity from:

1. SSH keys in `~/.ssh/` (ed25519 and RSA keys work with age)
2. Age identity file at `~/.config/age/identity.txt`
3. Custom path via `--identity` flag

## Encrypt a file

```bash
writ secrets encrypt .config/github/token
```

This encrypts the file to all recipients in `.age-recipients` and produces
`.config/github/token.age`. The original plaintext file is removed.

## Decrypt a file

```bash
# Output to stdout
writ secrets decrypt .config/github/token.age

# Write to file
writ secrets decrypt -o .config/github/token .config/github/token.age
```

## Edit encrypted files

Edit a secret in place without manually decrypting and re-encrypting:

```bash
writ secrets edit .config/github/token.age
```

This decrypts to a temporary file, opens `$EDITOR`, and re-encrypts when
the editor exits.

## Rekey secrets

When you add a new machine or revoke access, update `.age-recipients` and
re-encrypt all secrets:

```bash
writ secrets rekey
```

This decrypts and re-encrypts every `.age` file in the repository to match
the current recipients list. The operation is idempotent.

### Common rekey scenarios

| Scenario | Action |
|----------|--------|
| New machine | Add its public key to `.age-recipients`, run `rekey` |
| Revoked machine | Remove its key from `.age-recipients`, run `rekey` |
| Rotated keys | Update key in `.age-recipients`, run `rekey` |

## Upgrade secrets after source changes

When an encrypted source file changes (e.g., after pulling from git),
regenerate the decrypted copy:

```bash
writ upgrade noblefactor
```

This re-decrypts `.age` files and copies the updated plaintext to target
locations, respecting drift detection for locally modified files.

## Security model

| Property | Guarantee |
|----------|-----------|
| At rest | Encrypted with age (X25519 + ChaCha20-Poly1305) |
| In transit | Encrypted in git (only ciphertext committed) |
| On deploy | Decrypted copy written to target with restricted permissions |
| Key management | Recipients file in repo, identity stays on machine |

Secrets are never symlinked — they are always copied so the plaintext
never appears inside the git repository tree.
