# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# lint-tools.star - Check lint tool availability
#
# Show status of all required lint tools.

def run(command, ctx):
    """Check status of all required lint tools."""
    result = lint.ensure_tools()

    note("Checking lint tools...")
    for tool in result.tools:
        if tool.installed:
            succeed(tool.name + ": " + tool.path)
        else:
            error(tool.name + ": not installed")
            note("  Install: " + tool.install_cmd)

    if result.all_installed:
        succeed("All lint tools installed")
    else:
        print("")
        note("Install missing tools with:")
        for cmd in result.install_cmds:
            print("  " + cmd)
        fail("Missing required lint tools")
