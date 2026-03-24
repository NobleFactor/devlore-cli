# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# setup-tools.star - Show development tools status
#
# Display required tools and their installation status.

def run(command, ctx):
    """Show required tools and their installation status."""
    result = setup.tools()

    ui.note("Development tools for " + result.platform + ":")
    print("")

    for tool in result.tools:
        if tool.installed:
            ui.success(tool.name + ": " + tool.path)
        else:
            ui.error(tool.name + ": not installed")
            ui.note("  " + tool.description)
            ui.note("  Install: " + tool.install_cmd)
            ui.note("  Docs: " + tool.docs_url)
        print("")

    # Summary
    if result.all_installed:
        ui.success("All tools installed")
    else:
        print("")
        ui.note("Install missing tools:")
        for tool in result.tools:
            if not tool.installed:
                print("  " + tool.install_cmd)
        ui.fail(str(result.missing_count) + " tools missing")
