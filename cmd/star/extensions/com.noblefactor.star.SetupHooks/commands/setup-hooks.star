# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# setup-hooks.star - Install git hooks
#
# Install native git hooks for pre-commit checks.

def run(command, ctx):
    """Install native git hooks."""
    # Install pre-commit hook
    result = setup.install_hook(name="pre-commit")

    if result.success:
        if result.already_installed:
            ui.success("Git hooks already installed")
        else:
            ui.success("Installed pre-commit hook")
    else:
        ui.fail(result.message)
