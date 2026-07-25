# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# index.star - Generate index.yaml for knowledge domains
#
# This operation scans the knowledge/ directory in the target
# and generates index.yaml files for each domain.
#
# Asset types indexed:
#   - prompts/     -> prompts: [{name: ...}]
#   - schemas/     -> schemas: [{name: ...}]
#   - examples/    -> examples: [{name: ...}]
#   - transforms/  -> transforms: [{name: ...}]
#   - signatures/  -> signatures: [{name: ...}]
#   - slots/       -> slots: [{name: ...}]
#
# Usage:
#   star devlore knowledge index --target=/path/to/registry

# Asset type subdirectories to index
ASSET_TYPES = ["prompts", "schemas", "examples", "transforms", "signatures", "slots"]

def list_files(dir_path):
    """List all files in a directory (non-recursive)."""
    if not file.exists(dir_path):
        return []

    files = []
    for path in file.glob(file.join(dir_path, "*")):
        name = file.name(path)
        if not file.is_dir(path) and not name.startswith("."):
            files.append(name)
    return sorted(files)

def build_asset_entries(dir_path):
    """Build list of asset entries for a directory."""
    entries = []
    for filename in list_files(dir_path):
        entries.append({"name": filename})
    return entries

def build_index(domain_name, domain_path):
    """Build the complete index for a domain."""
    index = {"domain": domain_name}

    for asset_type in ASSET_TYPES:
        asset_dir = file.join(domain_path, asset_type)
        entries = build_asset_entries(asset_dir)
        if len(entries) > 0:
            index[asset_type] = entries

    return index


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        sibling = file.join("..", "devlore-registry")
        if file.is_dir(sibling):
            target = sibling
            ui.note("Using sibling registry: " + target)
        else:
            fail("--target required (no ../devlore-registry found)")
    if not file.is_dir(target):
        fail("Target path not found: " + target)
    return target


def run(command, ctx):
    """Main entry point."""
    target = _resolve_target(ctx)
    dry_run = ctx.args.get("dry_run", False)

    knowledge_dir = file.join(target, "knowledge")

    if not file.exists(knowledge_dir):
        fail("knowledge/ directory not found at " + knowledge_dir)
        return

    domains_processed = 0
    total_assets = 0

    for domain_path in file.glob(file.join(knowledge_dir, "*")):
        if not file.is_dir(domain_path):
            continue

        domain_name = file.name(domain_path)

        index = build_index(domain_name, domain_path)

        # Count assets
        asset_count = 0
        for asset_type in ASSET_TYPES:
            if asset_type in index:
                asset_count = asset_count + len(index[asset_type])

        if asset_count == 0:
            ui.note("Skipping empty domain: " + domain_name)
            continue

        index_content = yaml.encode(index)
        index_path = file.join(domain_path, "index.yaml")

        if dry_run:
            ui.note("Would write: " + index_path + " (" + str(asset_count) + " assets)")
            print(index_content)
            print("---")
        else:
            file.write_text(index_path, index_content)
            ui.succeed("Wrote: " + index_path + " (" + str(asset_count) + " assets)")

        domains_processed = domains_processed + 1
        total_assets = total_assets + asset_count

    ui.note("Indexed " + str(total_assets) + " assets across " + str(domains_processed) + " domain(s)")
