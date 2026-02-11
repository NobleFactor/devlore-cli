# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# sign-index.star - Generate and sign index.yaml
#
# This operation generates the package index and signs it with the
# release signing key (pmm-signing-dev or pmm-signing-prod).
#
# The signature proves the index is authentic NobleFactor content,
# preventing MITM attacks where an attacker substitutes malicious packages.
#
# See ADR-040: SSH Key Generation Ceremony for the signing protocol.
#
# Usage:
#   star devlore-registry sign index
#   star devlore-registry sign index --env=prod

def run(ctx):
    """Generate and sign index.yaml."""
    registry_path = ctx.args.get("path", ".")
    env = ctx.args.get("env", "dev")

    fail("Index signing not yet implemented")