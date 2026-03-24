# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# setup-check.star - Check setup status
#
# Check setup status without making changes.

def run(command, ctx):
    """Check setup status without making changes."""
    issues = []

    # Check tools
    tools_result = setup.tools()
    if tools_result.missing_count > 0:
        for tool in tools_result.tools:
            if not tool.installed:
                issues.append("Missing tool: " + tool.name)

    # Check git hooks
    hooks = setup.check_hook(name="pre-commit")
    if not hooks.installed:
        issues.append("Git hooks not installed (run: star setup hooks)")

    # Report
    if len(issues) == 0:
        ui.success("Repository setup is complete")
    else:
        for issue in issues:
            ui.error(issue)
        ui.fail(str(len(issues)) + " setup issues found")
