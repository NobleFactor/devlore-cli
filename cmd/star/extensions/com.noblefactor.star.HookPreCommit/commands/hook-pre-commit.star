# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# hook-pre-commit.star - Git pre-commit hook
#
# Run pre-commit checks for quality gates.

def run(command, ctx):
    """Run pre-commit checks."""
    # Run all linters
    failures = []

    # Go lint
    go_result = run_linter("go")
    if not go_result:
        failures.append("go")

    # Shell lint
    shell_result = run_linter("shell")
    if not shell_result:
        failures.append("shell")

    # Markdown lint
    md_result = run_linter("markdown")
    if not md_result:
        failures.append("markdown")

    # Summary
    if len(failures) == 0:
        ui.success("All pre-commit checks passed")
    else:
        ui.fail("Pre-commit checks failed: " + ", ".join(failures))

def run_linter(name):
    """Run a single linter, return True if passed."""
    if name == "go":
        return run_go_check()
    elif name == "shell":
        return run_shell_check()
    elif name == "markdown":
        return run_markdown_check()
    return True

def run_go_check():
    """Run Go lint checks, return True if passed."""
    tool = lint.ensure_tools()
    for t in tool.tools:
        if t.name == "golangci-lint" and not t.installed:
            ui.error("golangci-lint not installed")
            return False

    result = lint.go(path="./...", config="", skip_mod_tidy=False)

    if not result.mod_tidy_passed:
        ui.error("go.mod is not tidy - run 'go mod tidy'")

    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + " " + issue.linter + ": " + issue.message
        if issue.severity == "error":
            ui.error(msg)
        else:
            ui.warn(msg)

    return result.passed

def run_shell_check():
    """Run shell lint checks, return True if passed."""
    tool = lint.ensure_tools()
    has_shellcheck = False
    has_shfmt = False
    for t in tool.tools:
        if t.name == "shellcheck":
            has_shellcheck = t.installed
        if t.name == "shfmt":
            has_shfmt = t.installed

    if not has_shellcheck or not has_shfmt:
        # Skip silently if tools not installed
        return True

    result = lint.shell(path=".", severity="warning", indent=4)

    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + " SC" + str(issue.code) + ": " + issue.message
        if issue.level == "error":
            ui.error(msg)
        else:
            ui.warn(msg)

    for file_info in result.format_issues:
        ui.warn(file_info.file + " needs formatting - run 'shfmt -w -i 4'")

    return result.passed

def run_markdown_check():
    """Run markdown lint checks, return True if passed."""
    tool = lint.ensure_tools()
    for t in tool.tools:
        if t.name == "markdownlint-cli2" and not t.installed:
            # Skip silently if not installed
            return True

    result = lint.markdown(path=".", fix=False)

    for issue in result.issues:
        msg = issue.file + ":" + str(issue.line) + " " + issue.rule + ": " + issue.message
        if issue.severity == "error":
            ui.error(msg)
        else:
            ui.warn(msg)

    for issue in result.frontmatter_issues:
        ui.error(issue.file + ": " + issue.message)

    return result.lint_passed and result.frontmatter_passed
