# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# lint-go-style.star — Go style guidelines enforcement
#
# Orchestrator for the Go style linter. Discovers Go source files
# via the file provider (respects .gitignore), loads each into a
# SourceFile semantic tree, and runs check or fix. Config and job
# control only — all logic is in the provider.

def collect_files(paths):
    """Collect Go source files from the given paths."""
    files = []
    for p in paths:
        if file.is_file(resource=p):
            files.append(p)
        elif file.is_dir(resource=p):
            files.extend(sorted(file.find(p + "/**/*.go")))
        else:
            ui.fail(p + " is not a file or directory")
    return files

def run(command, ctx):
    """Enforce Go style guidelines on Go source files."""
    fix_mode = ctx.args.get("fix", False)
    paths = ctx.args.get("path", ["."])

    files = collect_files(paths)
    if not files:
        ui.succeed("No Go files found")
        return

    ui.note("Found " + str(len(files)) + " Go file(s)")

    if fix_mode:
        for f in files:
            ui.note("Fixing " + f)
            ast = goast.load_source_file(f)
            ast.cleanup()
            ast.save()
        ui.succeed("Fixed " + str(len(files)) + " file(s)")
    else:
        total_violations = 0
        for f in files:
            ui.note("Checking " + f)
            ast = goast.load_source_file(f)
            for v in ast.check_compliance:
                ui.warn(f + " [" + v.kind + "] " + v.message)
                total_violations += 1
        if total_violations > 0:
            ui.fail("Found " + str(total_violations) + " violation(s) in " + str(len(files)) + " file(s)")
        else:
            ui.succeed("All " + str(len(files)) + " file(s) compliant")
