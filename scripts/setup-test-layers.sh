#!/usr/bin/env bash
# setup-test-layers.sh — Create simulated base, team, and personal layer repos
# for ad-hoc e2e testing of multi-scope writ deploy.
#
# Usage:
#   ./scripts/setup-test-layers.sh [test-fsroot]
#
# Creates:
#   <test-fsroot>/
#     layers/
#       base/     — git repo with Home/ and System/ (foundational config)
#       team/     — git repo with Home/ (team-shared config)
#       personal/ — git repo with Home/ (user-specific config, drawn from ~/Workspace/Personal)
#     fake-home/  — empty directory to use as $HOME target
#     links/      — simulates ${XDG_DATA_HOME}/devlore/writ/layers/
#
# After running this script:
#   export HOME=<test-fsroot>/fake-home
#   export XDG_DATA_HOME=<test-fsroot>/fake-home/.local/share
#   writ deploy noblefactor
#
# Content sources:
#   - base:     synthetic foundational files (git config, vim, shell profile, one System/ file)
#   - team:     synthetic NobleFactor team config (git config, ssh config, overrides base gitignore for collision test)
#   - personal: real content drawn from ~/Workspace/Personal/Home/Configs (minus secrets)
#
# The personal layer maps Home/Configs/<project>/ → Home/<project>/ to match
# the directory structure writ expects. .Personal-secrets directories and
# private keys are excluded.

set -euo pipefail

PERSONAL_SRC="${HOME}/Workspace/Personal"
TEST_ROOT="${1:-/tmp/writ-test-layers}"

echo "=== Setting up test layers at ${TEST_ROOT} ==="

# Clean slate
rm -rf "${TEST_ROOT}"
mkdir -p "${TEST_ROOT}"/{layers,fake-home,links}

# ─── Base layer ──────────────────────────────────────────────────────────────

echo "--- Creating base layer ---"
BASE="${TEST_ROOT}/layers/base"
mkdir -p "${BASE}"

# Home/all — foundational config shared across all environments
mkdir -p "${BASE}/Home/all/.config/git"
cat >"${BASE}/Home/all/.config/git/ignore" <<'GITIGNORE'
.DS_Store
*.swp
*~
.idea/
.vscode/
GITIGNORE

cat >"${BASE}/Home/all/.config/git/attributes" <<'GITATTR'
*.sops  filter=sops diff=sops
*.age   filter=age  diff=age
GITATTR

mkdir -p "${BASE}/Home/all/.config/vim"
cat >"${BASE}/Home/all/.config/vim/vimrc" <<'VIMRC'
set nocompatible
set encoding=utf-8
set number
set ruler
set expandtab
set tabstop=4
set shiftwidth=4
syntax on
VIMRC

# Home/all — shell profile
cat >"${BASE}/Home/all/.profile" <<'PROFILE'
# Base profile — sourced by all shells
export EDITOR=vim
export PAGER=less
export LANG=en_US.UTF-8

# XDG Base Directory Specification
export XDG_CONFIG_HOME="${HOME}/.config"
export XDG_DATA_HOME="${HOME}/.local/share"
export XDG_STATE_HOME="${HOME}/.local/state"
export XDG_CACHE_HOME="${HOME}/.cache"

# Local bin
if [ -d "${HOME}/.local/bin" ]; then
    PATH="${HOME}/.local/bin:${PATH}"
fi
PROFILE

# Home/all-Darwin — macOS-specific base config
mkdir -p "${BASE}/Home/all-Darwin/.config"
cat >"${BASE}/Home/all-Darwin/.config/darwin-defaults.sh" <<'DARWIN'
#!/usr/bin/env bash
# Base Darwin defaults
defaults write NSGlobalDomain AppleShowAllExtensions -bool true
defaults write com.apple.finder ShowPathbar -bool true
DARWIN

# System/all — system-wide base config (would require elevation)
mkdir -p "${BASE}/System/all/etc"
cat >"${BASE}/System/all/etc/writ-base.conf" <<'SYSCONF'
# Base system configuration installed by writ
# This file is managed — do not edit manually
MANAGED_BY=writ
LAYER=base
SYSCONF

cd "${BASE}" && git init -q && git add -A && git commit -q -m "Initial base layer"

# ─── Team layer ──────────────────────────────────────────────────────────────

echo "--- Creating team layer ---"
TEAM="${TEST_ROOT}/layers/team"
mkdir -p "${TEAM}"

# Home/noblefactor — team-specific config
mkdir -p "${TEAM}/Home/noblefactor/.config/git"
cat >"${TEAM}/Home/noblefactor/.config/git/config.noblefactor" <<'GITCFG'
[user]
    email = dev@noblefactor.com
[commit]
    gpgsign = true
[pull]
    rebase = false
[init]
    defaultBranch = develop
GITCFG

mkdir -p "${TEAM}/Home/noblefactor/.ssh/config.d"
cat >"${TEAM}/Home/noblefactor/.ssh/config.d/noblefactor" <<'SSHCFG'
Host github.com-noblefactor
    HostName github.com
    User git
    IdentityFile ~/.ssh/id_ed25519_noblefactor
    IdentitiesOnly yes
SSHCFG

# Home/all — team overrides of base (collision: team wins)
mkdir -p "${TEAM}/Home/all/.config/git"
cat >"${TEAM}/Home/all/.config/git/ignore" <<'GITIGNORE'
# Team gitignore — shadows base
.DS_Store
*.swp
*~
.idea/
.vscode/
# NobleFactor additions
.devlore/
*.writ-backup
node_modules/
GITIGNORE

cd "${TEAM}" && git init -q && git add -A && git commit -q -m "Initial team layer"

# ─── Personal layer ──────────────────────────────────────────────────────────

echo "--- Creating personal layer ---"
PERSONAL="${TEST_ROOT}/layers/personal"
mkdir -p "${PERSONAL}"

# Draw non-secret content from ~/Workspace/Personal/Home/Configs
# Copy safe files only — skip .Personal-secrets, .ssh private keys
if [ -d "${PERSONAL_SRC}/Home/Configs" ]; then
    echo "    Drawing content from ${PERSONAL_SRC}/Home/Configs"

    # all — common personal config
    if [ -d "${PERSONAL_SRC}/Home/Configs/all" ]; then
        mkdir -p "${PERSONAL}/Home/all"
        # Copy .config (skip secrets)
        if [ -d "${PERSONAL_SRC}/Home/Configs/all/.config" ]; then
            cp -R "${PERSONAL_SRC}/Home/Configs/all/.config" "${PERSONAL}/Home/all/.config"
        fi
        # Copy .claude
        if [ -d "${PERSONAL_SRC}/Home/Configs/all/.claude" ]; then
            cp -R "${PERSONAL_SRC}/Home/Configs/all/.claude" "${PERSONAL}/Home/all/.claude"
        fi
        # Copy local/bin scripts
        if [ -d "${PERSONAL_SRC}/Home/Configs/all/local" ]; then
            mkdir -p "${PERSONAL}/Home/all/local"
            cp -R "${PERSONAL_SRC}/Home/Configs/all/local/bin" "${PERSONAL}/Home/all/local/bin" 2>/dev/null || true
        fi
    fi

    # all-Darwin — personal macOS config
    if [ -d "${PERSONAL_SRC}/Home/Configs/all-Darwin" ]; then
        mkdir -p "${PERSONAL}/Home/all-Darwin"
        for f in "${PERSONAL_SRC}/Home/Configs/all-Darwin"/.*; do
            fname="$(basename "$f")"
            case "${fname}" in
                . | .. | .Personal-secrets) continue ;;
                *) cp -R "$f" "${PERSONAL}/Home/all-Darwin/${fname}" 2>/dev/null || true ;;
            esac
        done
    fi

    # noblefactor — personal work config
    if [ -d "${PERSONAL_SRC}/Home/Configs/noblefactor" ]; then
        mkdir -p "${PERSONAL}/Home/noblefactor"
        for f in "${PERSONAL_SRC}/Home/Configs/noblefactor"/.* "${PERSONAL_SRC}/Home/Configs/noblefactor"/*; do
            fname="$(basename "$f")"
            case "${fname}" in
                . | .. | .Personal-secrets) continue ;;
                *) cp -R "$f" "${PERSONAL}/Home/noblefactor/${fname}" 2>/dev/null || true ;;
            esac
        done
    fi

    # noblefactor-Unix — personal Unix work config
    if [ -d "${PERSONAL_SRC}/Home/Configs/noblefactor-Unix" ]; then
        mkdir -p "${PERSONAL}/Home/noblefactor-Unix"
        for f in "${PERSONAL_SRC}/Home/Configs/noblefactor-Unix"/.* "${PERSONAL_SRC}/Home/Configs/noblefactor-Unix"/*; do
            fname="$(basename "$f")"
            case "${fname}" in
                . | .. | .Personal-secrets) continue ;;
                *) cp -R "$f" "${PERSONAL}/Home/noblefactor-Unix/${fname}" 2>/dev/null || true ;;
            esac
        done
    fi
else
    echo "    ${PERSONAL_SRC}/Home/Configs not found — creating synthetic personal content"

    mkdir -p "${PERSONAL}/Home/noblefactor/.config/git"
    cat >"${PERSONAL}/Home/noblefactor/.config/git/config.noblefactor" <<'GITCFG'
[user]
    name = David Noble
    email = david@noblefactor.com
GITCFG
fi

# Remove any accidentally copied secrets or private keys
find "${PERSONAL}" -name ".Personal-secrets" -type d -exec rm -rf {} + 2>/dev/null || true
find "${PERSONAL}" -name "*.sops" -delete 2>/dev/null || true
find "${PERSONAL}" -name "id_rsa" -delete 2>/dev/null || true
find "${PERSONAL}" -name "id_ed25519" -not -name "*.pub" -delete 2>/dev/null || true

cd "${PERSONAL}" && git init -q && git add -A && git commit -q -m "Initial personal layer"

# ─── Layer symlinks ──────────────────────────────────────────────────────────

echo "--- Creating layer symlinks ---"
LINKS="${TEST_ROOT}/links"
ln -s "${BASE}" "${LINKS}/base"
ln -s "${TEAM}" "${LINKS}/team"
ln -s "${PERSONAL}" "${LINKS}/personal"

# ─── Fake home directory ─────────────────────────────────────────────────────

echo "--- Preparing fake home ---"
FAKE_HOME="${TEST_ROOT}/fake-home"
mkdir -p "${FAKE_HOME}/.local/share/devlore/writ/layers"
mkdir -p "${FAKE_HOME}/.local/state/devlore"
mkdir -p "${FAKE_HOME}/.config/devlore"

# Link layers into the fake XDG location
ln -s "${BASE}" "${FAKE_HOME}/.local/share/devlore/writ/layers/base"
ln -s "${TEAM}" "${FAKE_HOME}/.local/share/devlore/writ/layers/team"
ln -s "${PERSONAL}" "${FAKE_HOME}/.local/share/devlore/writ/layers/personal"

# ─── Summary ─────────────────────────────────────────────────────────────────

echo ""
echo "=== Test layers ready ==="
echo ""
echo "Structure:"
echo "  ${TEST_ROOT}/"
echo "    layers/"
echo "      base/     $(cd "${BASE}" && git rev-parse --short HEAD)"
echo "      team/     $(cd "${TEAM}" && git rev-parse --short HEAD)"
echo "      personal/ $(cd "${PERSONAL}" && git rev-parse --short HEAD)"
echo "    fake-home/"
echo "    links/ → symlinks to layers"
echo ""
echo "Layer contents:"
for layer in base team personal; do
    echo "  ${layer}:"
    for scope in System Home; do
        dir="${TEST_ROOT}/layers/${layer}/${scope}"
        if [ -d "${dir}" ]; then
            count=$(find "${dir}" -type f | wc -l | tr -d ' ')
            projects=$(ls "${dir}" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
            echo "    ${scope}/  ${count} files  [${projects}]"
        fi
    done
done
echo ""
echo "To test:"
echo "  export HOME=${FAKE_HOME}"
echo "  export XDG_DATA_HOME=${FAKE_HOME}/.local/share"
echo "  export XDG_STATE_HOME=${FAKE_HOME}/.local/state"
echo "  export XDG_CONFIG_HOME=${FAKE_HOME}/.config"
echo "  writ deploy noblefactor"
echo ""
echo "To inspect the fake home after deploy:"
echo "  find ${FAKE_HOME} -type l -o -type f | sort"
