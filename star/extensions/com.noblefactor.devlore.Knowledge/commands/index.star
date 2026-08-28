# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# index.star - Generate index.yaml for knowledge domains
#
# This operation scans knowledge/ in the target and updates each domain's index.yaml.
#
# It UPDATES rather than rebuilds. An earlier version listed each asset directory and wrote
# {name: filename} per entry, discarding everything else in the file. That was invisible while the
# workflow was failing; its first successful run (devlore-registry#80) deleted the concepts section
# from two domains, providers from a third, and every slot's package/description/platforms/
# install_types. See docs/plans/knowledge-index-preserves-metadata.md.
#
# The rule now: anything this command cannot classify is an ERROR, not an omission. Its silence
# used to be indistinguishable from success -- the output still parsed, so validate-yaml-schemas
# stayed green while content was being deleted.
#
# Asset types:
#   prompts/ schemas/ examples/ transforms/ signatures/ slots/ concepts/ providers/ bindings/
#
# Usage:
#   star devlore knowledge index --target=/path/to/registry [--dry_run=true]

ASSET_TYPES = [
    "prompts",
    "schemas",
    "examples",
    "transforms",
    "signatures",
    "slots",
    "concepts",
    "providers",
    "bindings",
]

def list_files(dir_path):
    """List file names directly in a directory, sorted. Empty if the directory is absent."""
    if not file.exists(dir_path):
        return []

    files = []
    for path in file.glob(file.join(dir_path, "*")):
        name = file.name(path)
        if not file.is_dir(path) and not name.startswith("."):
            files.append(name)
    return sorted(files)

def list_subdirs(domain_path):
    """List subdirectory names directly under a domain, sorted."""
    dirs = []
    for path in file.glob(file.join(domain_path, "*")):
        name = file.name(path)
        if file.is_dir(path) and not name.startswith("."):
            dirs.append(name)
    return sorted(dirs)

def load_existing(index_path):
    """Parse the domain's current index.yaml, or an empty dict when there is none."""
    if not file.exists(index_path):
        return {}
    # yaml.decode, not yaml.parse: parse returns a yaml.Resource whose only attributes are
    # equal/resource_base/validate, so the document's data is unreachable through it. decode
    # returns a plain dict.
    decoded = yaml.decode(file.read_text(index_path))
    if type(decoded) != "dict":
        return {}
    return decoded

def entry_names(entries):
    """Names from a list of index entries, skipping anything malformed."""
    names = []
    for e in entries:
        if type(e) == "dict" and "name" in e:
            names.append(e["name"])
    return names

def audit_domain(domain_name, domain_path, existing):
    """Every way this domain's index and its directories can disagree.

    Returns a list of human-readable problems. A non-empty list fails the run: each entry is
    something only a person can resolve, and guessing is how metadata gets deleted.
    """
    problems = []
    subdirs = list_subdirs(domain_path)

    # A directory nobody taught us about. Its contents are invisible today --
    # package-authoring/bindings/ sat unindexed this way.
    for d in subdirs:
        if d not in ASSET_TYPES:
            problems.append("directory '" + d + "/' is not a known asset type")

    # A file loose in the domain root belongs to no asset type, so nothing indexes it.
    for path in file.glob(file.join(domain_path, "*")):
        name = file.name(path)
        if file.is_dir(path) or name.startswith(".") or name == "index.yaml":
            continue
        problems.append("file '" + name + "' is outside any asset-type directory")

    for key in existing:
        if key == "domain":
            continue

        # A section naming a type we do not know.
        if key not in ASSET_TYPES:
            problems.append("section '" + key + "' is not a known asset type")
            continue

        # A section whose directory is gone. Dropping it silently is what this rewrite exists
        # to prevent.
        if key not in subdirs:
            problems.append("section '" + key + "' has no '" + key + "/' directory")
            continue

        if type(existing[key]) != "list":
            problems.append("section '" + key + "' is not a list")
            continue

        # An entry whose file is absent. It may have been deleted, or it may have moved --
        # signatures/dotfile-systems.yaml was the latter -- and only a person knows which.
        present = list_files(file.join(domain_path, key))
        for name in entry_names(existing[key]):
            if name not in present:
                problems.append("'" + key + "/" + name + "' is indexed but no such file exists")

    # A directory holding assets that the index never mentions.
    for d in subdirs:
        if d in ASSET_TYPES and d not in existing and len(list_files(file.join(domain_path, d))) > 0:
            problems.append("directory '" + d + "/' has assets but no '" + d + ":' section")

    return problems

def merge_entries(names, existing_entries):
    """Keep each existing entry whole; add {name} for a file that has none.

    The metadata on an entry -- purpose, description, source_system, and whatever a slot carries --
    is hand-written. This command's job is to track which files exist, not to have opinions about
    what they mean.
    """
    by_name = {}
    for e in existing_entries:
        if type(e) == "dict" and "name" in e:
            by_name[e["name"]] = e

    merged = []
    for name in names:
        if name in by_name:
            merged.append(by_name[name])
        else:
            merged.append({"name": name})
    return merged

def build_index(domain_name, domain_path, existing):
    """The updated index: same sections, same metadata, entries tracking the files on disk."""
    index = {"domain": domain_name}

    for asset_type in ASSET_TYPES:
        names = list_files(file.join(domain_path, asset_type))
        if len(names) == 0:
            continue

        prior = []
        if asset_type in existing and type(existing[asset_type]) == "list":
            prior = existing[asset_type]

        index[asset_type] = merge_entries(names, prior)

    return index


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        sibling = file.join("..", "devlore-registry")
        if file.is_dir(sibling):
            target = sibling
            note("Using sibling registry: " + target)
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

    # Keyed by name and sorted by it: file.glob yields path values, which do not order.
    by_name = {}
    for domain_path in file.glob(file.join(knowledge_dir, "*")):
        if file.is_dir(domain_path):
            by_name[file.name(domain_path)] = domain_path
    domains = sorted(by_name.keys())

    # Audit every domain before writing any of them. A partial write across a tree that is
    # inconsistent somewhere else is harder to reason about than no write at all, and the report
    # is more use to a person whole than one problem at a time.
    problems = []
    plans = []

    for domain_name in domains:
        domain_path = by_name[domain_name]
        index_path = file.join(domain_path, "index.yaml")
        existing = load_existing(index_path)

        found = audit_domain(domain_name, domain_path, existing)
        for p in found:
            problems.append(domain_name + ": " + p)

        plans.append((domain_name, domain_path, index_path, existing))

    if len(problems) > 0:
        # The problems go into the error, not only into the log. A caller that sees the failure but
        # not stdout -- a CI step summary, a test -- would otherwise be told that something was
        # wrong without being told what, which is barely better than the silence this replaces.
        for p in problems:
            note("  " + p)

        detail = ""
        for p in problems:
            detail = detail + "\n  - " + p

        fail(
            "refusing to write: " + str(len(problems)) +
            " problem(s) across " + str(len(domains)) + " domain(s)." + detail +
            "\nEach is a decision this command will not make for you: resolve it, or teach " +
            "ASSET_TYPES about a directory that belongs.",
        )
        return

    domains_written = 0
    total_assets = 0
    unchanged = 0

    for entry in plans:
        domain_name = entry[0]
        domain_path = entry[1]
        index_path = entry[2]
        existing = entry[3]

        index = build_index(domain_name, domain_path, existing)

        asset_count = 0
        for asset_type in ASSET_TYPES:
            if asset_type in index:
                asset_count = asset_count + len(index[asset_type])

        # yaml.encode drops comments, so a domain's hand-written header cannot survive a
        # regeneration. Say so in the file rather than leaving it to be rediscovered.
        index_content = (
            "# Generated by: star devlore knowledge index\n" +
            "# Entry metadata (purpose, description, source_system, ...) is preserved across runs\n" +
            "# and is edited by hand. This command only tracks which files exist.\n" +
            yaml.encode(index)
        )

        # A no-op is worth saying out loud: it is the evidence that a run changed nothing, which is
        # what re-enabling this in CI has to demonstrate.
        if file.exists(index_path) and file.read_text(index_path) == index_content:
            unchanged = unchanged + 1
            total_assets = total_assets + asset_count
            continue

        if dry_run:
            note("Would write: " + index_path + " (" + str(asset_count) + " assets)")
            print(index_content)
            print("---")
        else:
            file.write_text(index_path, index_content)
            succeed("Wrote: " + index_path + " (" + str(asset_count) + " assets)")

        domains_written = domains_written + 1
        total_assets = total_assets + asset_count

    note(
        "Indexed " + str(total_assets) + " assets across " + str(len(domains)) + " domain(s); " +
        str(domains_written) + " changed, " + str(unchanged) + " unchanged",
    )
