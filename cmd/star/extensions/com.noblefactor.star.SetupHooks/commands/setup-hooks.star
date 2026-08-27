# SPDX-License-Identifier: Apache-2.0
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
            succeed("Git hooks already installed")
        else:
            succeed("Installed pre-commit hook")
    else:
        fail(result.message)
