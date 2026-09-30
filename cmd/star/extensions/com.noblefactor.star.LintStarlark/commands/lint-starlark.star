# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# lint-starlark.star - Starlark resolution checks
#
# Resolves every plan.* call against the action surface devlore actually has, and checks each package
# phase script against its lifecycle's phase order.
#
# There is no tool to install. The checker is embedded, which is the point: buildifier is installable
# today and would have caught none of the seven defects that motivated this, because they are resolution
# errors against devlore's own provider surface rather than syntax (devlore-cli#721).
#
# File discovery uses the file provider, so it respects .gitignore, plus the exclusions in star.yaml.

def matches_pattern(path, pattern):
    """Report whether a path matches an exclusion pattern, as a prefix or a whole path segment."""
    if path == pattern or path.startswith(pattern + "/"):
        return True
    if ("/" + pattern + "/") in ("/" + path):
        return True
    if pattern.startswith("**/"):
        suffix = pattern[3:]
        return path.endswith("/" + suffix) or path == suffix
    return False

def is_excluded(path, exclude_patterns):
    """Report whether a path matches any exclusion pattern."""
    for pattern in exclude_patterns:
        if matches_pattern(path, pattern):
            return True
    return False

def collect_files(paths, exclude_patterns):
    """Collect Starlark files from the given paths, honoring exclusions."""
    files = []
    for p in paths:
        if file.is_file(path = p):
            if not is_excluded(p, exclude_patterns):
                files.append(p)
        elif file.is_dir(path = p):
            found = [entry.source_path.rel() for entry in file.find(p + "/**/*.star")]
            files.extend(sorted([f for f in found if not is_excluded(f, exclude_patterns)]))
    return files

def run(command, ctx):
    """Check Starlark files for dead plan.* references and phase-script mistakes."""
    paths = ctx.args.get("path", ["."])

    # lint.all calls every sibling with fix=, so the flag has to exist. Nothing here is auto-fixable: a
    # dead plan.* call needs a human to decide what was meant.
    if ctx.args.get("fix", False):
        warn("lint.starlark has nothing to fix automatically; checking only")

    cfg = config.get
    exclude_patterns = list(cfg.lint.starlark.exclude)

    star_files = collect_files(paths, exclude_patterns)
    if not star_files:
        succeed("No Starlark files found")
        return

    note("Found " + str(len(star_files)) + " Starlark file(s)")

    result = lint.starlark(files = star_files)

    # Every finding is an error. A dead plan.* reference cannot execute and a misnamed phase script is
    # never opened, so there is no severity ladder here to put one of them below the gate.
    for issue in result.issues:
        error(issue.file + ":" + str(issue.line) + ": " + issue.message)

    if result.passed:
        succeed("Starlark lint passed (" + str(result.files_checked) + " files)")
    else:
        fail("Starlark lint failed: " + str(result.issue_count) + " issue(s) in " +
             str(result.files_checked) + " file(s)")
