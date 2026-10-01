# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# lint-copyright.star - Copyright header checking and fixing
#
# The header is configuration. `lint.copyright.header` carries the whole thing as a literal, comment markers
# excluded, and this file supplies the marker from the file's language. There are no template fields: the
# identifier is typed into the header where a reader can see it, so nothing infers it.
#
# That replaced a LICENSE_PATTERNS table and a detect_license that matched the LICENSE file by substring --
# "Apache License" appears in the Apache-2.0 text and in anything merely mentioning it, and dict order decided
# the winner when two matched. Same defect as devlore-cli#997, in a third place (devlore-cli#994).

# =============================================================================
# Language Detection and Comment Styles
# =============================================================================

# Extension to comment style mapping
# Note: Config files like .yaml, .toml, .json are excluded as they
# typically don't require copyright headers
COMMENT_STYLES = {
    # Hash comments
    ".go": "//",
    ".star": "#",
    ".sh": "#",
    ".bash": "#",
    ".zsh": "#",
    ".py": "#",
    ".rb": "#",
    ".pl": "#",
    ".tf": "#",
    # Slash comments
    ".js": "//",
    ".ts": "//",
    ".jsx": "//",
    ".tsx": "//",
    ".c": "//",
    ".h": "//",
    ".cpp": "//",
    ".cc": "//",
    ".hpp": "//",
    ".java": "//",
    ".kt": "//",
    ".rs": "//",
    ".swift": "//",
    ".cs": "//",
    ".scala": "//",
    ".groovy": "//",
    ".gradle": "//",
    ".proto": "//",
    ".dart": "//",
    ".zig": "//",
    # Double-dash comments
    ".sql": "--",
    ".lua": "--",
    ".hs": "--",
    ".elm": "--",
    # Other
    ".el": ";;",
    ".lisp": ";;",
    ".clj": ";;",
    ".vim": "\"",
    ".erl": "%",
    ".tex": "%",
}

def get_comment_style(path):
    """Return comment prefix for the given file type."""
    for ext, style in COMMENT_STYLES.items():
        if path.endswith(ext):
            return style
    return None

def get_file_extension(path):
    """Get file extension from path."""
    parts = path.split(".")
    if len(parts) > 1:
        return "." + parts[-1]
    return ""

# =============================================================================
# The Header
# =============================================================================

# SPDX_PATTERN and COPYRIGHT_PATTERN are gone, not tightened. With the header configured as a literal there is
# nothing to pattern-match: it is rendered once per comment style and compared. That is what makes check
# require exactly what fix produces -- one string, used in both directions, so they cannot disagree.
#
# What they let through, and why tightening them was never the answer: the copyright line was matched by
# `holder not in found_holder`, a substring test that four different notices satisfied; the spacing after the
# marker was `\s*`, so `//SPDX-License-Identifier:Apache-2.0` passed; and nothing looked at the blank line
# before the code, which in Go is the difference between a file header and the package doc comment
# (devlore-cli#997).

def commented(header, comment):
    """Prefix each line of the configured header with a language's comment marker."""
    lines = []

    for line in header.split("\n"):
        lines.append(comment + " " + line if line else comment)

    return "\n".join(lines)

def skip_count(lines):
    """Return how many leading lines the header must follow, for a script carrying a shebang."""
    if len(lines) > 0 and lines[0].startswith("#!"):
        if len(lines) > 1 and lines[1].strip() == "":
            return 2
        return 1
    return 0

# =============================================================================
# Header Checking
# =============================================================================

def check_file(path, expected):
    """Check whether a file carries the configured header."""
    comment = get_comment_style(path)
    if comment == None:
        return {"ok": True, "message": "", "skipped": True}

    lines = file.read_text(path).split("\n")
    start = skip_count(lines)
    wanted = expected.split("\n")

    if len(lines) < start + len(wanted):
        return {"ok": False, "message": "the header is missing", "skipped": False}

    for i in range(len(wanted)):
        if lines[start + i] != wanted[i]:
            return {
                "ok": False,
                "message": "line " + str(start + i + 1) + " is\n      " + lines[start + i] +
                           "\n    and must be\n      " + wanted[i],
                "skipped": False,
            }

    return {"ok": True, "message": "", "skipped": False}

# =============================================================================
# Header Fixing
# =============================================================================

def fix_file(path, expected):
    """Replace a file's header with the configured one."""
    comment = get_comment_style(path)
    if comment == None:
        return {"fixed": False, "error": "Unknown file type"}

    lines = file.read_text(path).split("\n")

    shebang = ""
    start_line = skip_count(lines)
    if start_line > 0:
        shebang = lines[0] + "\n\n"

    # The header already present is the leading run of comment lines and the blank lines after it. This
    # replaced a scan bounded to five lines that matched the two deleted regexes -- a bound with no stated
    # reason, which mis-handled any file whose leading comment block ran longer.
    header_end = start_line
    while header_end < len(lines) and lines[header_end].startswith(comment):
        header_end += 1
    while header_end < len(lines) and lines[header_end].strip() == "":
        header_end += 1

    new_content = shebang + expected + "\n\n" + "\n".join(lines[header_end:])
    if not new_content.endswith("\n"):
        new_content = new_content + "\n"

    file.write_text(path, new_content)
    return {"fixed": True, "error": ""}

# =============================================================================
# Pattern Matching
# =============================================================================

def matches_pattern(path, pattern):
    """Check if a path matches a glob pattern."""
    # Simple glob matching for common patterns
    # Handles: **, *, and literal matches

    # Normalize path separators
    path = path.replace("\\", "/")
    pattern = pattern.replace("\\", "/")

    # Strip leading ./ from path
    if path.startswith("./"):
        path = path[2:]

    # Handle ** patterns (e.g., vendor/**)
    if "**" in pattern:
        # vendor/** matches vendor/anything (relative or absolute paths)
        base = pattern.replace("/**", "")
        if path.startswith(base + "/") or path == base:
            return True
        # Also match when base appears as a path segment in absolute paths
        if ("/" + base + "/") in path:
            return True
        # **/vendor matches anything/vendor
        if pattern.startswith("**/"):
            suffix = pattern[3:]
            if path.endswith("/" + suffix) or path == suffix:
                return True
            # Also match intermediate directories
            if ("/" + suffix + "/") in ("/" + path):
                return True
        return False

    # Handle simple * patterns
    if "*" in pattern:
        # Split on * and check if parts match
        parts = pattern.split("*")
        if len(parts) == 2:
            return path.startswith(parts[0]) and path.endswith(parts[1])

    # Literal match
    return path == pattern or path.startswith(pattern + "/")

def is_excluded(path, exclude_patterns):
    """Check if path matches any exclusion pattern."""
    for pattern in exclude_patterns:
        if matches_pattern(path, pattern):
            return True
    return False

# =============================================================================
# File Collection
# =============================================================================

def collect_source_files(paths, exclude_patterns):
    """Collect source files from paths, respecting .gitignore and exclude patterns."""
    all_files = []

    for path in paths:
        # Collect files by extension
        # file.find supports ** recursive patterns and respects .gitignore by default
        for ext in COMMENT_STYLES.keys():
            pattern = path + "/**/*" + ext
            files = file.find(pattern)
            for f in files:
                # file.find returns file.Resource values; derive the relative path
                # string for exclusion matching and the downstream string ops
                # (get_comment_style, read_text/write_text accept the path string).
                file_path = f.source_path.rel()
                if not is_excluded(file_path, exclude_patterns):
                    all_files.append(file_path)

    return all_files

# =============================================================================
# Command Entry Point
# =============================================================================

def run(command, ctx):
    """Check or fix copyright headers in source files."""
    fix_mode = ctx.args.get("fix", False)
    paths = ctx.args.get("path", ["."])

    # Load config
    cfg = config.get
    copyright_cfg = cfg.lint.copyright

    if not copyright_cfg.enabled:
        warn("Copyright checking is disabled in star.yaml")
        warn("Add 'lint.copyright.enabled: true' to enable")
        return

    # The header is configuration, and the only source of it. An unset header is an error rather than a
    # default, because a header belongs to the repository being checked rather than to the linter checking it
    # -- this extension is embedded in the star binary and runs on other people's code (devlore-cli#994).
    header = copyright_cfg.header
    if not header:
        fail("lint.copyright.header is not set. It carries the header text, without comment markers")

    header = header.rstrip("\n")

    # Rendered once per comment style, not once per file. There are a handful of styles and 1,001 files.
    expected_by_comment = {}
    for comment in COMMENT_STYLES.values():
        if comment not in expected_by_comment:
            expected_by_comment[comment] = commented(header, comment)

    # Get explicit exclude patterns from config (in addition to .gitignore)
    exclude_patterns = list(copyright_cfg.exclude)

    # Collect files (respects .gitignore automatically + config excludes)
    files = collect_source_files(paths, exclude_patterns)

    if len(files) == 0:
        note("No source files found")
        return

    note("Checking " + str(len(files)) + " source files...")

    if fix_mode:
        fixed = []
        errors = []

        for f in files:
            comment = get_comment_style(f)
            if comment == None:
                continue

            expected = expected_by_comment[comment]

            check_result = check_file(f, expected)
            if check_result["skipped"] or check_result["ok"]:
                continue

            fix_result = fix_file(f, expected)
            if fix_result["fixed"]:
                fixed.append(f)
            else:
                errors.append({"file": f, "message": fix_result["error"]})

        if len(fixed) > 0:
            succeed("Fixed " + str(len(fixed)) + " files:")
            for f in fixed:
                note("  " + f)

        if len(errors) > 0:
            for e in errors:
                error(e["file"] + ": " + e["message"])
            fail("Could not fix " + str(len(errors)) + " files")
        elif len(fixed) == 0:
            succeed("All files have correct copyright headers")
    else:
        issues = []

        for f in files:
            comment = get_comment_style(f)
            if comment == None:
                continue

            result = check_file(f, expected_by_comment[comment])
            if not result["skipped"] and not result["ok"]:
                issues.append({"file": f, "message": result["message"]})

        if len(issues) == 0:
            succeed("All " + str(len(files)) + " files have correct copyright headers")
        else:
            for issue in issues:
                error(issue["file"] + ": " + issue["message"])
            fail("Found " + str(len(issues)) + " files with copyright issues (run with --fix to repair)")
