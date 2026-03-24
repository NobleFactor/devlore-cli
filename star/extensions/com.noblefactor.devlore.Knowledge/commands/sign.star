# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# sign.star - Generate and sign knowledge index
#
# This operation generates the knowledge index and signs it with the
# release signing key (pmm-signing-dev or pmm-signing-prod).
#
# The signature proves the index is authentic NobleFactor content,
# preventing MITM attacks where an attacker substitutes malicious packages.
#
# See ADR-040: SSH Key Generation Ceremony for the signing protocol.
#
# Usage:
#   star devlore knowledge sign
#   star devlore knowledge sign --env=prod


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        sibling = file.join("..", "devlore-registry")
        if file.is_dir(sibling):
            target = sibling
            ui.note("Using sibling registry: " + target)
        else:
            fail("--target required (no ../devlore-registry found)")
    if not file.is_dir(target):
        fail("Target path not found: " + target)
    return target


def run(command, ctx):
    """Generate and sign index.yaml."""
    target = _resolve_target(ctx)
    env = ctx.args.get("env", "dev")

    fail("Index signing not yet implemented")
