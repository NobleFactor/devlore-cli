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
#   star devlore-registry sign package --path=packages/ripgrep
#   star devlore-registry sign package --path=packages/ripgrep --env=prod

def run(ctx):
    """Sign a lore package with release key."""
    package_path = ctx.args.get("path", "")
    env = ctx.args.get("env", "dev")

    if not package_path:
        fail("--path is required")

    fail("Package signing not yet implemented: " + package_path)