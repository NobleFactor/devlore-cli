# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# lint-markdown.star - Markdown lint checks
#
# Run markdownlint and frontmatter validation on markdown files.
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
    """Collect markdown files from the given paths."""
    files = []
    for p in paths:
        if file.is_file(resource=p):
            files.append(p)
        elif file.is_dir(resource=p):
            files.extend(sorted(file.find(p + "/**/*.md")))
    return files

def run(command, ctx):
    """Run markdownlint and frontmatter check on markdown files."""
    paths = ctx.args.get("path", ["."])
    fix = ctx.args.get("fix", False)

    # Check tool is installed
    ensure_tool_installed("markdownlint-cli2")

    # Sync config if needed
    config.sync()

    # Discover files (respects .gitignore)
    md_files = collect_files(paths)
    if not md_files:
        ui.succeed("No markdown files found")
        return

    ui.note("Found " + str(len(md_files)) + " markdown file(s)")

    # Run markdownlint on discovered files
    result = lint.markdown(files=md_files, fix=fix)

    # Report issues
    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + " " + issue.rule + ": " + issue.message
        if issue.severity == "error":
            ui.error(msg)
        else:
            ui.warn(msg)

    for issue in result.frontmatter_issues:
        ui.error(issue.file + ": " + issue.message)

    # Summary
    if result.lint_passed and result.frontmatter_passed:
        ui.succeed("Markdown lint passed (" + str(result.files_checked) + " files)")
    else:
        msg = "Markdown lint failed:"
        if result.issue_count > 0:
            msg = msg + " " + str(result.issue_count) + " lint issues"
        if len(result.frontmatter_issues) > 0:
            msg = msg + " " + str(len(result.frontmatter_issues)) + " frontmatter issues"
        ui.fail(msg)
