#!/bin/bash
# Install dotfiles using GNU Stow
set -e
cd "$(dirname "$0")"
for pkg in bash vim tmux git; do
    stow -v -t ~ "$pkg"
done
echo "Dotfiles installed!"
