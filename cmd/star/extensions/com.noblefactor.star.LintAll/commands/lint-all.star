# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# lint-all.star - Run all configured linters
#
# Uses the commands API to discover and run all sibling lint commands.

def run(command, ctx):
    """Run all configured linters."""
    fix = ctx.args.get("fix", False)

    # None, not ["."], when the caller named no path. Forwarding a default of "." overrode each linter's own
    # default, and lint.go's is "./..." for a reason: golangci-lint reads "." as the root package alone, which
    # holds no .go files, so `lint all` failed with "no go files to analyze" while `lint go ./...` passed.
    # A linter knows its own corpus better than the aggregator does.
    paths = ctx.args.get("path", None)

    # Get all sibling lint commands (lint.go, lint.shell, etc.)
    siblings = commands.siblings()

    if len(siblings) == 0:
        warn("No lint commands found")
        return

    # Track results
    failures = []
    passed = []

    # Run each sibling command
    for cmd in siblings:
        # Extract short name (e.g., "go" from "lint.go")
        short_name = cmd.name.split(".")[-1]
        note("=== " + short_name.upper() + " ===")

        # Check if command should be skipped based on config
        if short_name == "copyright":
            cfg = config.get
            if not cfg.lint.copyright.enabled:
                note("Skipped (disabled in star.yaml)")
                continue

        # lint.tools doesn't take paths, and neither does a run where the caller named none.
        if short_name == "tools" or paths == None:
            result = cmd.run(fix=fix)
        else:
            result = cmd.run(fix=fix, path=paths)

        if result.passed:
            passed.append(cmd.name)
        else:
            # Report why. Without this the aggregator says "failed" and nothing else, so a linter that
            # could not be INVOKED is indistinguishable from one that ran and found problems -- and the
            # cause, which commands.run already returns, is discarded.
            if result.error:
                error(result.error)
            failures.append(cmd.name)

    # Summary
    note("")
    note("=== SUMMARY ===")

    if len(passed) > 0:
        for name in passed:
            succeed(name.split(".")[-1] + ": passed")

    if len(failures) > 0:
        for name in failures:
            error(name.split(".")[-1] + ": failed")
        fail("Linters failed: " + ", ".join([n.split(".")[-1] for n in failures]))
    else:
        succeed("All " + str(len(passed)) + " linters passed")
