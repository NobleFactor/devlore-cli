# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# lint-copyright.star - Copyright header checking and fixing
#
# Ensures all source files have correct SPDX license headers.
# Configuration is loaded from star.yaml lint.copyright section.
#
# Pure Starlark implementation using file and regexp receivers.

# =============================================================================
# License Detection
# =============================================================================

LICENSE_PATTERNS = {
    "MIT": r"(?i)MIT\s+License|Permission is hereby granted, free of charge",
    "Apache-2.0": r"(?i)Apache\s+License.*Version\s+2\.0|www\.apache\.org/licenses/LICENSE-2\.0",
    "BSD-3-Clause": r"(?i)BSD\s+3-Clause|Redistribution and use in source and binary forms",
    "BSD-2-Clause": r"(?i)BSD\s+2-Clause",
    "GPL-3.0": r"(?i)GNU\s+GENERAL\s+PUBLIC\s+LICENSE.*Version\s+3",
    "GPL-2.0": r"(?i)GNU\s+GENERAL\s+PUBLIC\s+LICENSE.*Version\s+2",
    "LGPL-3.0": r"(?i)GNU\s+LESSER\s+GENERAL\s+PUBLIC\s+LICENSE.*Version\s+3",
    "MPL-2.0": r"(?i)Mozilla\s+Public\s+License.*2\.0",
    "ISC": r"(?i)ISC\s+License|Permission to use, copy, modify, and/or distribute",
    "Unlicense": r"(?i)This is free and unencumbered software released into the public domain",
}

def detect_license(license_path):
    """Detect SPDX license identifier from LICENSE file."""
    if not file.exists(license_path):
        return {"detected": False, "license": "", "error": "LICENSE file not found"}

    content = file.read_text(license_path)

    for spdx_id, pattern in LICENSE_PATTERNS.items():
        if regexp.match(pattern, content):
            return {"detected": True, "license": spdx_id, "error": ""}

    return {"detected": False, "license": "", "error": "Could not identify license type"}

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
# Header Patterns
# =============================================================================

# Pattern to match SPDX header line
SPDX_PATTERN = r"^(//|#|--|;;|\"|%)\s*SPDX-License-Identifier:\s*(\S+)"

# Pattern to match copyright line
COPYRIGHT_PATTERN = r"^(//|#|--|;;|\"|%)\s*Copyright\s+([^.]+)"

def build_expected_header(license, holder, comment):
    """Build the expected copyright header."""
    return comment + " SPDX-License-Identifier: " + license + "\n" + comment + " Copyright " + holder + ". All rights reserved."

# =============================================================================
# Header Checking
# =============================================================================

def check_file(path, license, holder):
    """Check if a file has the correct copyright header."""
    comment = get_comment_style(path)
    if comment == None:
        return {"ok": True, "message": "", "skipped": True}

    content = file.read_text(path)
    lines = content.split("\n")

    # Handle shebang for scripts
    start_line = 0
    if len(lines) > 0 and lines[0].startswith("#!"):
        start_line = 1
        if len(lines) > 1 and lines[1].strip() == "":
            start_line = 2

    # Check for SPDX line
    if len(lines) <= start_line:
        return {"ok": False, "message": "Missing SPDX license header", "skipped": False}

    spdx_line = lines[start_line]
    spdx_match = regexp.find_submatch(SPDX_PATTERN, spdx_line)

    if not spdx_match:
        return {"ok": False, "message": "Missing SPDX license header", "skipped": False}

    found_license = spdx_match[2]
    if found_license != license:
        return {"ok": False, "message": "Wrong license: expected " + license + ", found " + found_license, "skipped": False}

    # Check for copyright line
    if len(lines) <= start_line + 1:
        return {"ok": False, "message": "Missing copyright holder line", "skipped": False}

    copyright_line = lines[start_line + 1]
    copyright_match = regexp.find_submatch(COPYRIGHT_PATTERN, copyright_line)

    if not copyright_match:
        return {"ok": False, "message": "Missing copyright holder line", "skipped": False}

    found_holder = copyright_match[2]
    if holder not in found_holder:
        return {"ok": False, "message": "Wrong holder: expected '" + holder + "', found '" + found_holder + "'", "skipped": False}

    return {"ok": True, "message": "", "skipped": False}

# =============================================================================
# Header Fixing
# =============================================================================

def fix_file(path, license, holder):
    """Fix the copyright header in a file."""
    comment = get_comment_style(path)
    if comment == None:
        return {"fixed": False, "error": "Unknown file type"}

    content = file.read_text(path)
    lines = content.split("\n")
    expected_header = build_expected_header(license, holder, comment)

    # Handle shebang
    shebang = ""
    start_line = 0
    if len(lines) > 0 and lines[0].startswith("#!"):
        shebang = lines[0] + "\n\n"
        start_line = 1
        if len(lines) > 1 and lines[1].strip() == "":
            start_line = 2

    # Find existing header to replace
    header_end = start_line
    for i in range(start_line, min(start_line + 5, len(lines))):
        line = lines[i]
        if regexp.match(SPDX_PATTERN, line) or regexp.match(COPYRIGHT_PATTERN, line):
            header_end = i + 1
        elif line.strip() == "" and header_end > start_line:
            header_end = i + 1
            break
        elif line.strip() != "" and not line.startswith(comment):
            break

    # Build new content
    remaining_lines = lines[header_end:]
    while len(remaining_lines) > 0 and remaining_lines[0].strip() == "":
        remaining_lines = remaining_lines[1:]

    new_content = shebang + expected_header + "\n\n" + "\n".join(remaining_lines)
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
                # Apply explicit exclude patterns from config
                if not is_excluded(f, exclude_patterns):
                    all_files.append(f)

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
        ui.warn("Copyright checking is disabled in star.yaml")
        ui.warn("Add 'lint.copyright.enabled: true' to enable")
        return

    # Detect license if set to "auto"
    license = copyright_cfg.license
    if license == "auto":
        result = detect_license("LICENSE")
        if result["detected"]:
            license = result["license"]
            ui.note("Detected license: " + license)
        else:
            ui.fail("Could not detect license from LICENSE file. Set lint.copyright.license in star.yaml")

    holder = copyright_cfg.holder
    if not holder:
        ui.fail("Copyright holder not configured. Set lint.copyright.holder in star.yaml")

    # Get explicit exclude patterns from config (in addition to .gitignore)
    exclude_patterns = list(copyright_cfg.exclude)

    # Collect files (respects .gitignore automatically + config excludes)
    files = collect_source_files(paths, exclude_patterns)

    if len(files) == 0:
        ui.note("No source files found")
        return

    ui.note("Checking " + str(len(files)) + " source files...")

    if fix_mode:
        fixed = []
        errors = []

        for f in files:
            check_result = check_file(f, license, holder)
            if check_result["skipped"] or check_result["ok"]:
                continue

            fix_result = fix_file(f, license, holder)
            if fix_result["fixed"]:
                fixed.append(f)
            else:
                errors.append({"file": f, "message": fix_result["error"]})

        if len(fixed) > 0:
            ui.success("Fixed " + str(len(fixed)) + " files:")
            for f in fixed:
                ui.note("  " + f)

        if len(errors) > 0:
            for e in errors:
                ui.error(e["file"] + ": " + e["message"])
            ui.fail("Could not fix " + str(len(errors)) + " files")
        elif len(fixed) == 0:
            ui.success("All files have correct copyright headers")
    else:
        issues = []

        for f in files:
            result = check_file(f, license, holder)
            if not result["skipped"] and not result["ok"]:
                issues.append({"file": f, "message": result["message"]})

        if len(issues) == 0:
            ui.success("All " + str(len(files)) + " files have correct copyright headers")
        else:
            for issue in issues:
                ui.error(issue["file"] + ": " + issue["message"])
            ui.fail("Found " + str(len(issues)) + " files with copyright issues (run with --fix to repair)")
