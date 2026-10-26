#!/bin/sh

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# scenario-container-run.sh - Inside a distribution's image: install what the scenarios need, then run them (#995)
#
# Run by scenario-in-container.sh, as root, in /src, the checkout. POSIX sh, since an Alpine image has no bash until
# this installs it. It prints the image's os-release, installs bash, GNU make, git, GNU getopt, curl and tar with the
# image's own package manager, installs Go from go.dev checked against the SHA-256 go.dev publishes, makes a user with
# HOST_UID and HOST_GID, and runs make test-scenario as that user.

set -o errexit -o nounset

GO_VERSION=1.27.3
GO_SHA256=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40 # go1.27.3.linux-amd64.tar.gz, from go.dev

echo "--- /etc/os-release"
cat /etc/os-release

echo "--- the tools"
if command -v apt-get >/dev/null 2>&1; then
    apt-get update --quiet
    DEBIAN_FRONTEND=noninteractive apt-get install --yes --quiet bash ca-certificates curl git make tar util-linux
elif command -v dnf >/dev/null 2>&1; then
    # --allowerasing: an image's curl-minimal gives way to curl rather than conflicting with it.
    dnf install --assumeyes --quiet --allowerasing bash ca-certificates curl git make tar util-linux shadow-utils
elif command -v apk >/dev/null 2>&1; then
    apk add --quiet bash ca-certificates curl git make tar util-linux-misc shadow
else
    echo "scenario-container-run.sh: no apt-get, dnf or apk in this image" >&2
    exit 69
fi

echo "--- Go ${GO_VERSION}"
curl --fail --silent --show-error --location --output /tmp/go.tar.gz \
    "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c -
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz

echo "--- the user"
groupadd --gid "${HOST_GID}" scenario 2>/dev/null || true
useradd --uid "${HOST_UID}" --gid "${HOST_GID}" --create-home --shell /bin/bash scenario

echo "--- make test-scenario"
# shellcheck disable=SC2016 # ${PATH} is the scenario user's, expanded by its shell, not this one
su scenario -c 'export PATH="/usr/local/go/bin:${PATH}"; cd /src && make test-scenario'
