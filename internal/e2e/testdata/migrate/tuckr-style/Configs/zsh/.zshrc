# shellcheck shell=bash
# Zsh configuration
export ZSH="$HOME/.oh-my-zsh"
# shellcheck disable=SC2034
ZSH_THEME="robbyrussell"
# shellcheck disable=SC2034
plugins=(git docker kubectl)
# shellcheck source=/dev/null
source $ZSH/oh-my-zsh.sh
