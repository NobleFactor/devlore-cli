# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# setup-config.star - Initialize configuration
#
# Initialize star.yaml and sync tool configurations.

def run(command, ctx):
    """Initialize star.yaml and sync tool configurations."""
    result = setup.init_config()

    if result.star_yaml_created:
        succeed("Created " + result.star_yaml_path)
    else:
        note(result.star_yaml_path + " already exists")

    if len(result.configs_synced) > 0:
        for cfg in result.configs_synced:
            succeed("Synced " + cfg)
    else:
        note("Tool configs already up to date")
