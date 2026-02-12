# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# sign-package.star - Sign a lore package with release key
#
# This operation signs a lore package (PMM) with the release signing key.
# The signature proves the package is authentic NobleFactor content.
#
# See ADR-040: SSH Key Generation Ceremony for the signing protocol.
#
# Usage:
#   star devlore knowledge sign package --target=../devlore-registry
#   star devlore knowledge sign package --target=../devlore-registry --env=prod


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        sibling = file.join("..", "devlore-registry")
        if file.is_directory(sibling):
            target = sibling
            note("Using sibling registry: " + target)
        else:
            fail("--target required (no ../devlore-registry found)")
    if not file.is_directory(target):
        fail("Target path not found: " + target)
    return target


def run(ctx):
    """Sign a lore package with release key."""
    target = _resolve_target(ctx)
    env = ctx.args.get("env", "dev")

    fail("Package signing not yet implemented")
