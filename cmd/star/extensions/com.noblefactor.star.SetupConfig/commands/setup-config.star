# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# setup-config.star - Initialize configuration
#
# Initialize star.yaml and sync tool configurations.

def run(command, ctx):
    """Initialize star.yaml and sync tool configurations."""
    result = setup.init_config()

    if result.star_yaml_created:
        ui.succeed("Created " + result.star_yaml_path)
    else:
        ui.note(result.star_yaml_path + " already exists")

    if len(result.configs_synced) > 0:
        for cfg in result.configs_synced:
            ui.succeed("Synced " + cfg)
    else:
        ui.note("Tool configs already up to date")
