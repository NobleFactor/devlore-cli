#!/bin/bash
# One-time package installation
if command -v brew &>/dev/null; then
    brew install nvim tmux
fi
