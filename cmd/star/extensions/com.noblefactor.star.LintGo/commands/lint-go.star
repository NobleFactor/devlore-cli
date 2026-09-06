# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# lint-go.star - Go lint checks
#
# Run golangci-lint with go mod tidy verification.

def check_tool(name):
    """Check if a tool is installed and return its status."""
    result = lint.ensure_tools()
    for tool in result.tools:
        if tool.name == name:
            return tool
    return None

def ensure_tool_installed(name):
    """Ensure a tool is installed, fail with install instructions if not."""
    tool = check_tool(name)
    if tool and not tool.installed:
        fail(name + " is not installed\n  Install: " + tool.install_cmd)
    return tool

def run(command, ctx):
    """Run golangci-lint on Go code."""
    paths = ctx.args.get("path", ["./..."])
    config = ctx.args.get("golangci-config", "")
    skip_mod_tidy = ctx.args.get("skip_mod_tidy", False)

    # Check tool is installed
    ensure_tool_installed("golangci-lint")

    note("Running Go lint checks on " + " ".join(paths))

    # Run go mod tidy check first (unless skipped)
    if not skip_mod_tidy:
        note("Checking go.mod tidy...")

    result = lint.go(paths=paths, config=config, skip_mod_tidy=skip_mod_tidy)

    # Report mod tidy status
    if not skip_mod_tidy:
        if result.mod_tidy_passed:
            succeed("go.mod is tidy")
        else:
            error("go.mod is not tidy")
            if result.mod_tidy_details:
                for line in result.mod_tidy_details.split("\n"):
                    if line:
                        note("  " + line)

    # Note if config was created
    if result.config_created:
        succeed("Created .golangci.yaml with NobleFactor defaults")

    # Report golangci-lint issues
    note("Running golangci-lint...")
    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + ":" + str(issue.column)
        msg = msg + " " + issue.linter + ": " + issue.message
        if issue.severity == "error":
            error(msg)
        else:
            warn(msg)

    # Summary
    if result.lint_passed:
        succeed("No golangci-lint issues found")
    else:
        warn("Found " + str(result.total_count) + " lint issues (" + str(result.error_count) + " errors, " + str(result.warning_count) + " warnings)")

    # Final pass/fail
    if result.passed:
        succeed("All Go lint checks passed")
    else:
        msgs = []
        if not result.mod_tidy_passed:
            msgs.append("go.mod not tidy")
        if not result.lint_passed:
            msgs.append(str(result.total_count) + " lint issues")
        fail("Go lint failed: " + ", ".join(msgs))
