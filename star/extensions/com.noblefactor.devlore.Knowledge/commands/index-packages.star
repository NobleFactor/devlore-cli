# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# index-packages.star - Generate package indexes
#
# This operation scans the packages/ directory in the target
# and generates two index files:
#
#   packages/index.yaml           - Package listing with metadata
#   packages/cross-reference.yaml - Native package name -> lore package mappings
#
# Usage:
#   star devlore knowledge index packages --target=/path/to/registry

def parse_lifecycle(content):
    """Parse lifecycle.yaml content and extract all fields."""
    data = yaml.decode(content)
    if data == None:
        return None

    return {
        "name": data.get("name", ""),
        "version": data.get("version", ""),
        "description": data.get("description", ""),
        "homepage": data.get("homepage", ""),
        "license": data.get("license", ""),
        "maintainer": data.get("maintainer", ""),
        "platforms": data.get("platforms", []),
        "provides": data.get("provides", []),
        "aliases": data.get("aliases", []),
        "tags": data.get("tags", []),
        "signatures": data.get("signatures", {}),
    }

def scan_packages(packages_dir):
    """Scan packages directory and collect metadata."""
    packages = []

    for entry in file.list(packages_dir):
        if not entry.is_dir:
            continue

        lifecycle_path = file.join(entry.path, "lifecycle.yaml")
        if not file.exists(lifecycle_path):
            warn("Skipping " + entry.name + " (no lifecycle.yaml)")
            continue

        content = file.read(lifecycle_path)
        pkg = parse_lifecycle(content)
        if pkg == None:
            warn("Skipping " + entry.name + " (invalid lifecycle.yaml)")
            continue

        # Add directory name for reference
        pkg["dir"] = entry.name

        # Check for README
        readme_path = file.join(entry.path, "README.md")
        pkg["has_readme"] = file.exists(readme_path)

        # List available platform variants
        variants = []
        for subentry in file.list(entry.path):
            if subentry.is_dir and not subentry.name.startswith("."):
                variants.append(subentry.name)
        pkg["variants"] = variants

        packages.append(pkg)
        note("Found: " + pkg["name"] + " v" + pkg["version"])

    return packages

def build_index(packages):
    """Build the package index structure."""
    # Sort packages by name
    sorted_pkgs = sorted(packages, key=lambda p: p["name"])

    # Remove signatures from index entries (they go in package-resolution.yaml)
    index_pkgs = []
    for pkg in sorted_pkgs:
        entry = dict(pkg)
        entry.pop("signatures", None)
        index_pkgs.append(entry)

    return {
        "version": "1",
        "generated": "star devlore knowledge index packages",
        "count": len(index_pkgs),
        "packages": index_pkgs,
    }

def build_package_resolution(packages):
    """Build the package resolution index (native name -> lore package)."""
    # manager -> native_name -> lore_package
    resolution = {}

    for pkg in packages:
        lore_package = pkg["name"]
        signatures = pkg.get("signatures", {})

        for manager, names in signatures.items():
            if type(names) != "list":
                warn(lore_package + " signatures." + manager + " is not a list")
                continue

            if manager not in resolution:
                resolution[manager] = {}

            for name in names:
                if name in resolution[manager]:
                    existing = resolution[manager][name]
                    if existing != lore_package:
                        warn(manager + ":" + name + " maps to both " + existing + " and " + lore_package)
                resolution[manager][name] = lore_package

    # Sort managers for consistent output
    sorted_resolution = {}
    for manager in sorted(resolution.keys()):
        sorted_resolution[manager] = resolution[manager]

    return sorted_resolution


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        sibling = file.join("..", "devlore-registry")
        if file.is_directory(sibling):
            target = sibling
            note("Using sibling registry: " + target)
        else:
            fail("--target required (no ../devlore-registry found)")
    if not file.is_directory(target):
        fail("Target path not found: " + target)
    return target


def run(ctx):
    """Main entry point."""
    target = _resolve_target(ctx)

    packages_dir = file.join(target, "packages")

    if not file.exists(packages_dir):
        fail("packages/ directory not found at " + packages_dir)
        return

    note("Scanning packages in " + packages_dir)
    packages = scan_packages(packages_dir)

    if len(packages) == 0:
        warn("No packages found")
        return

    # Build and write package index
    index = build_index(packages)
    index_path = file.join(packages_dir, "index.yaml")
    file.write(index_path, yaml.encode(index))
    success("Wrote: " + index_path)

    # Build and write cross-reference
    xref = build_package_resolution(packages)
    xref_path = file.join(packages_dir, "cross-reference.yaml")

    # Count mappings
    total_mappings = 0
    for manager, names in xref.items():
        total_mappings = total_mappings + len(names)

    if total_mappings > 0:
        file.write(xref_path, yaml.encode(xref))
        success("Wrote: " + xref_path)
        note(str(len(xref)) + " managers, " + str(total_mappings) + " mappings")
    else:
        note("No cross-reference mappings found")

    note("Indexed " + str(len(packages)) + " package(s)")
