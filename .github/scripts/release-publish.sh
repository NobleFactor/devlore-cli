#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# release-publish.sh - Publish a release run's assets: create the release, complete it, or leave it as it is
#
# Usage: release-publish.sh <version> <commit> <prerelease: true|false> <directory of assets>
#
# Run by .github/workflows/release.yaml with GH_TOKEN and RELEASE_NOTES set; scripts/Test-Release.sh runs it against a
# stand-in gh. A rerun of a release run produces the release the first run made (#980; ruled 2026-10-23, "B"):
#
#   - no release has the version: it is created at the commit, with every asset;
#   - the release has every asset, by name: nothing changes;
#   - the release lacks one: every asset is uploaded, replacing those there, so the archives and the checksums file
#     come from one build. The build is reproducible, so a replaced asset is the same bytes.
#
# --target tags the commit the build came from. Without it GitHub creates the tag at the head of the default branch,
# develop, so a build from main or release/* would be tagged on develop's head (#947). A v* tag already exists, and
# gh leaves it where it is.

set -o errexit -o nounset -o pipefail

if (($# != 4)); then
    echo "usage: release-publish.sh <version> <commit> <prerelease: true|false> <directory of assets>" >&2
    exit 64
fi
version=$1 commit=$2 prerelease=$3 directory=$4

# The assets, in name order: every file the build left in the directory, and nothing else.
assets=()
while IFS= read -r -d '' asset; do
    assets+=("$asset")
done < <(find "$directory" -mindepth 1 -maxdepth 1 -type f -print0 | LC_ALL=C sort -z)
if ((${#assets[@]} == 0)); then
    echo "release-publish.sh: no assets in ${directory}" >&2
    exit 66
fi

if ! present=$(gh release view "$version" --json assets --jq '.assets[].name' 2>/dev/null); then
    flags=()
    if [[ "$prerelease" == true ]]; then
        flags=(--prerelease)
    fi
    gh release create "$version" --target "$commit" --title "$version" --notes "${RELEASE_NOTES}" \
        ${flags[@]+"${flags[@]}"} "${assets[@]}"
    echo "created ${version} with ${#assets[@]} assets"
    exit 0
fi

missing=0
for asset in "${assets[@]}"; do
    grep --quiet --line-regexp --fixed-strings "$(basename "$asset")" <<<"$present" || missing=$((missing + 1))
done

if ((missing == 0)); then
    echo "${version} has every asset; nothing changed"
    exit 0
fi

gh release upload "$version" --clobber "${assets[@]}"
echo "${version} lacked ${missing} asset(s); replaced all ${#assets[@]} from this build"
