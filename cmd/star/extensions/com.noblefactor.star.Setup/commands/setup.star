# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# setup.star - Run all setup tasks
#
# Ensures a repository is ready for development with all required tools,
# hooks, and configuration in place.

def run(command, ctx):
    """Run all setup tasks."""
    ui.note("Setting up repository...")

    # Check tools first
    tools_result = setup.tools()
    if tools_result.missing_count > 0:
        ui.warn(str(tools_result.missing_count) + " tools missing (run 'star setup tools' for details)")
    else:
        ui.success("All " + str(len(tools_result.tools)) + " tools installed")

    # Initialize config
    config_result = setup.init_config()
    if config_result.star_yaml_created:
        ui.success("Created star.yaml")
    for cfg in config_result.configs_synced:
        ui.success("Synced " + cfg)

    # Install native git hooks
    hooks_result = setup.install_hook(name="pre-commit")
    if hooks_result.success:
        if hooks_result.already_installed:
            ui.note("Git hooks already installed")
        else:
            ui.success("Installed pre-commit hook")
    else:
        ui.warn(hooks_result.message)

    # Final status
    if tools_result.missing_count > 0:
        ui.warn("Setup complete with warnings - install missing tools")
    else:
        ui.success("Repository setup complete")
