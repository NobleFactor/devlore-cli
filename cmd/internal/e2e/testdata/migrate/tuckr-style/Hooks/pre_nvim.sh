#!/bin/bash
# Pre-hook for neovim setup
echo "Installing neovim plugins..."
nvim --headless "+Lazy! sync" +qa
