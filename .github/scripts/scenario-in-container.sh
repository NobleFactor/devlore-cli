#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# scenario-in-container.sh - Run make test-scenario on a Linux distribution's own image (#995)
#
# Usage: .github/scripts/scenario-in-container.sh <image>
#
# Run by ci.yaml's distribution job, from the root of a checkout, on a runner with docker. It runs the image with
# docker run over the checkout, not as a container job: GitHub's JavaScript actions, checkout and setup-go among them,
# cannot run inside an Alpine image. scenario-container-run.sh does the work inside, as root, then runs the scenarios as
# a user whose uid and gid are the caller's, so the checkout stays the caller's.

set -o errexit -o nounset -o pipefail

if (($# != 1)); then
    echo "usage: scenario-in-container.sh <image>" >&2
    exit 64
fi

docker run --rm \
    --volume "${PWD}:/src" \
    --workdir /src \
    --env HOST_UID="$(id -u)" \
    --env HOST_GID="$(id -g)" \
    "$1" /bin/sh .github/scripts/scenario-container-run.sh
