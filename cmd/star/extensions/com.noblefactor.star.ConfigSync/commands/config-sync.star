# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# config-sync.star - Sync tool configurations
#
# Write tool-specific config files from star.yaml.

def run(command, ctx):
    """Sync tool-specific config files from star.yaml."""
    result = config.sync()

    if result.files_generated == 0:
        note("No tool configs to sync (no config sections in star.yaml)")
        return

    if result.golangci_lint:
        succeed("Generated " + result.golangci_lint)
    if result.markdown_lint:
        succeed("Generated " + result.markdown_lint)

    succeed("Synced " + str(result.files_generated) + " config file(s)")
