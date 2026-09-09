# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_pkg_purl.star — Dry-run: a package named by its canonical purl plans (#813).
#
# The provider emits `purl.String()` as a resource's identity and could not read that form back, so a manifest
# written in the canonical spelling was refused as an unknown package manager named "pkg". This plans one.
#
# The purl type must be a manager the host platform registers, and registration is per OS: brew on Darwin,
# winget on Windows, the distro's own on Linux — apt on the Debian family, which is what CI runs.

purl_type = {"darwin": "brew", "windows": "winget"}.get(platform.os(), "apt")

graph = plan.assemble_definition([
    plan.pkg.install(packages=["pkg:" + purl_type + "/curl"], manager="", cask=False),
    plan.pkg.installed(name="pkg:" + purl_type + "/curl"),
])
t.expect_unit_count(2)
