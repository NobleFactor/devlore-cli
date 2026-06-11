# SPDX-License-Identifier: SSPL-1.0
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.

# run.star — Run a Starlark test script via the devlore-test binary.
#
# Resolves the devlore-test binary via a 3-tier lookup:
#   1. --tool-path flag (explicit override)
#   2. DEVLORE_TEST_TOOL_PATH environment variable
#   3. Extension config test.tool_path (default: build/devlore-test)
#
# The resolved path is relative to the git worktree fsroot (the same fsroot
# that star uses for extension discovery).

def resolve_tool_path(ctx):
    """Resolve the devlore-test binary path via 3-tier lookup."""

    # Tier 1: explicit --tool-path flag
    tool_path = ctx.args.get("tool-path", "")
    if tool_path:
        return tool_path

    # Tier 2: environment variable
    env_path = ctx.env.get("DEVLORE_TEST_TOOL_PATH", "")
    if env_path:
        return env_path

    # Tier 3: extension config (default: build/devlore-test relative to worktree fsroot)
    config_path = ctx.config.get("test.tool_path", "build/devlore-test")
    workspace = ctx.env.get("GIT_WORKSPACE_ROOT", "")
    if workspace:
        return file.join(parts=[workspace, config_path])

    return config_path

def build_args(ctx, tool_path):
    """Build the devlore-test command arguments."""
    args = [tool_path, ctx.args["script"]]

    if ctx.args.get("dry-run", False):
        args.append("--dry-run")
    if ctx.args.get("trace", False):
        args.append("--trace")

    return args

def run(command, ctx):
    """Run a Starlark test script that plans and executes a graph."""
    tool_path = resolve_tool_path(ctx)

    if not file.exists(blob=tool_path):
        ui.fail("devlore-test binary not found: " + tool_path)
        ui.note("Run 'make build' to compile the binary")
        return

    args = build_args(ctx, tool_path)
    ui.note("Running: " + " ".join(args))

    # Execute the binary and parse JSON output.
    # Requires host.exec() receiver — planned for noblefactor-ops.
    # For now, run devlore-test directly from the command line:
    #   build/devlore-test --script <path>
    ui.warn("Direct execution from star not yet wired — run the binary manually:")
    ui.note("  " + " ".join(args))
