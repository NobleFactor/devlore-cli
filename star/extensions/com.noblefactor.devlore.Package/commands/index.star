# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# index.star - Generate package indexes
#
# This operation scans the packages/ directory in the target
# and generates two index files:
#
#   packages/index.yaml           - Package listing with metadata
#   packages/cross-reference.yaml - Native package name -> lore package mappings
#
# Usage:
#   star devlore package index --target=/path/to/registry

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
    # Collect package dirs and variant dirs in a single walk.
    # Depth 1 = package dir, depth 2 = variant dir.
    pkg_dirs = []     # [(name, abs_path)]
    variant_map = {}  # pkg_name -> [variant_name]

    def collect(entry):
        if not entry.is_dir:
            return
        depth = entry.path.count("/") + 1
        if depth == 1:
            pkg_dirs.append((entry.name, file.join(packages_dir, entry.path)))
        elif depth == 2 and not entry.name.startswith("."):
            parent = entry.path.split("/")[0]
            if parent not in variant_map:
                variant_map[parent] = []
            variant_map[parent].append(entry.name)
            return "skip"
        elif depth > 2:
            return "skip"

    file.walk_tree(root=packages_dir, callback=collect)

    packages = []
    for pkg_name, pkg_path in pkg_dirs:
        lifecycle_path = file.join(pkg_path, "lifecycle.yaml")
        if not file.exists(lifecycle_path):
            ui.warn("Skipping " + pkg_name + " (no lifecycle.yaml)")
            continue

        content = file.read(lifecycle_path)
        pkg = parse_lifecycle(content)
        if pkg == None:
            ui.warn("Skipping " + pkg_name + " (invalid lifecycle.yaml)")
            continue

        pkg["dir"] = pkg_name
        pkg["has_readme"] = file.exists(file.join(pkg_path, "README.md"))
        pkg["variants"] = variant_map.get(pkg_name, [])

        packages.append(pkg)
        ui.note("Found: " + pkg["name"] + " v" + pkg["version"])

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
                ui.warn(lore_package + " signatures." + manager + " is not a list")
                continue

            if manager not in resolution:
                resolution[manager] = {}

            for name in names:
                if name in resolution[manager]:
                    existing = resolution[manager][name]
                    if existing != lore_package:
                        ui.warn(manager + ":" + name + " maps to both " + existing + " and " + lore_package)
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
            ui.note("Using sibling registry: " + target)
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

    ui.note("Scanning packages in " + packages_dir)
    packages = scan_packages(packages_dir)

    if len(packages) == 0:
        ui.warn("No packages found")
        return

    # Build and write package index
    index = build_index(packages)
    index_path = file.join(packages_dir, "index.yaml")
    file.write(index_path, yaml.encode(index))
    ui.success("Wrote: " + index_path)

    # Build and write cross-reference
    xref = build_package_resolution(packages)
    xref_path = file.join(packages_dir, "cross-reference.yaml")

    # Count mappings
    total_mappings = 0
    for manager, names in xref.items():
        total_mappings = total_mappings + len(names)

    if total_mappings > 0:
        file.write(xref_path, yaml.encode(xref))
        ui.success("Wrote: " + xref_path)
        ui.note(str(len(xref)) + " managers, " + str(total_mappings) + " mappings")
    else:
        ui.note("No cross-reference mappings found")

    ui.note("Indexed " + str(len(packages)) + " package(s)")
