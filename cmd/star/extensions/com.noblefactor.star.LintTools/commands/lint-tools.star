# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# lint-tools.star - Check lint tool availability
#
# Show status of all required lint tools.

def run(command, ctx):
    """Check status of all required lint tools."""
    result = lint.ensure_tools()

    ui.note("Checking lint tools...")
    for tool in result.tools:
        if tool.installed:
            ui.succeed(tool.name + ": " + tool.path)
        else:
            ui.error(tool.name + ": not installed")
            ui.note("  Install: " + tool.install_cmd)

    if result.all_installed:
        ui.succeed("All lint tools installed")
    else:
        print("")
        ui.note("Install missing tools with:")
        for cmd in result.install_cmds:
            print("  " + cmd)
        ui.fail("Missing required lint tools")
