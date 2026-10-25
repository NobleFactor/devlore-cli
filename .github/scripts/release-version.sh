#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# release-version.sh - Print a release run's version, pre-release flag and channel, as GitHub output lines
#
# Run by .github/workflows/release.yaml in the checkout of the commit it builds, with GitHub's GITHUB_REF and
# GITHUB_REF_NAME; its output is appended to GITHUB_OUTPUT. scripts/Test-Release.sh runs it the same way.
#
# A branch build's timestamp is its commit's committer time in UTC, never the clock, so a rerun of a run computes the
# version the first run did and publishes nothing new (#980). A v* tag's version is the tag.
#
# The binaries carry their channel (#947), decided here with GitHub's prerelease flag so the two cannot disagree: a
# develop build follows develop, every one a pre-release; a build from main or release/* follows release and takes its
# pre-releases; a v* tag is a release and takes none. PRERELEASE is both the flag and the stamp, so a binary takes
# pre-releases exactly when it is one.

set -o errexit -o nounset -o pipefail

stamp=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y%m%d%H%M%S)

case "${GITHUB_REF}" in
    refs/tags/v*)
        version="${GITHUB_REF_NAME}"
        prerelease=false
        channel=release
        ;;
    refs/heads/main)
        version="v0.1.0-rc.${stamp}"
        prerelease=true
        channel=release
        ;;
    refs/heads/release/*)
        version="v${GITHUB_REF_NAME#release/}-rc.${stamp}"
        prerelease=true
        channel=release
        ;;
    refs/heads/develop)
        version="v0.1.0-dev.${stamp}"
        prerelease=true
        channel=develop
        ;;
    *)
        echo "release-version.sh: no release is made from ${GITHUB_REF}" >&2
        exit 64
        ;;
esac

printf 'version=%s\nprerelease=%s\nchannel=%s\n' "$version" "$prerelease" "$channel"
