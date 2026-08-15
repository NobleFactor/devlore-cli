#!/bin/bash
# Custom installation script
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DOTFILES_DIR="$(dirname "$SCRIPT_DIR")"

echo "Installing dotfiles..."

# Link bash configs
ln -sf "$DOTFILES_DIR/configs/bash/bashrc" ~/.bashrc

# Link vim configs
mkdir -p ~/.vim
ln -sf "$DOTFILES_DIR/configs/vim/vimrc" ~/.vimrc

echo "Done!"
