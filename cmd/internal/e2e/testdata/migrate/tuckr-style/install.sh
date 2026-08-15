#!/bin/bash
# Install script that uses tuckr
set -e
echo "Setting up dotfiles with tuckr..."
tuckr add all
tuckr set nvim zsh
echo "Done!"
