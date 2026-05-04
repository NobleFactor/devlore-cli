# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# lint-shell.star - Shell script lint checks
#
# Run shellcheck and shfmt on shell scripts.
# File discovery uses the file provider (respects .gitignore).

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
        ui.fail(name + " is not installed\n  Install: " + tool.install_cmd)
    return tool

def collect_files(paths):
    """Collect shell script files from the given paths."""
    files = []
    for p in paths:
        if file.is_file(resource=p):
            files.append(p)
        elif file.is_dir(resource=p):
            files.extend(sorted(file.find(p + "/**/*.sh")))
            files.extend(sorted(file.find(p + "/**/*.bash")))
            files.extend(sorted(file.find(p + "/**/*.zsh")))
    return files

def run(command, ctx):
    """Run shellcheck and shfmt on shell scripts."""
    paths = ctx.args.get("path", ["."])
    severity = ctx.args.get("severity", "warning")
    indent = ctx.args.get("indent", 4)

    # Check tools are installed
    ensure_tool_installed("shellcheck")
    ensure_tool_installed("shfmt")

    # Discover files (respects .gitignore)
    shell_files = collect_files(paths)
    if not shell_files:
        ui.succeed("No shell files found")
        return

    ui.note("Found " + str(len(shell_files)) + " shell file(s)")

    # Run shell lint on discovered files
    result = lint.shell(files=shell_files, severity=severity, indent=indent)

    # Report shellcheck issues
    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + ":" + str(issue.column)
        msg = msg + " SC" + str(issue.code) + ": " + issue.message
        if issue.level == "error":
            ui.error(msg)
        elif issue.level == "warning":
            ui.warn(msg)
        else:
            ui.note(msg)

    # Report formatting issues
    for file_info in result.format_issues:
        ui.warn(file_info.file + " needs formatting")
        if file_info.diff:
            lines = file_info.diff.split("\n")
            for line in lines[:10]:
                ui.note("  " + line)
            if len(lines) > 10:
                ui.note("  ... (" + str(len(lines) - 10) + " more lines)")

    # Summary
    if result.passed:
        ui.succeed("Shell lint passed (" + str(result.files_checked) + " files)")
    else:
        msg = "Shell lint failed:"
        if result.error_count > 0 or result.warning_count > 0:
            msg = msg + " " + str(result.error_count) + " errors, " + str(result.warning_count) + " warnings"
        if len(result.format_issues) > 0:
            msg = msg + " " + str(len(result.format_issues)) + " files need formatting"
        ui.fail(msg)
