#!/bin/bash
# Custom uninstallation script
set -e
echo "Removing dotfile symlinks..."
rm -f ~/.bashrc ~/.vimrc
echo "Done!"
