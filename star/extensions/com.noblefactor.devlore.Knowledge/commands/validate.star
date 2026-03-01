# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# validate.star - Validate knowledge artifact YAML files against JSON schemas
#
# This operation validates knowledge YAML files in the target against their
# corresponding JSON schemas.
#
# Usage:
#   star devlore knowledge validate
#   star devlore knowledge validate --target=/path/to/registry

# Schema definitions: type -> (file_pattern, schema_path)
SCHEMAS = {
    "knowledge.index": {
        "pattern": "knowledge/*/index.yaml",
        "schema": "schemas/knowledge.index.json",
    },
}

def find_files(target, pattern):
    """Find files matching a glob-like pattern."""
    if "*" not in pattern:
        path = file.join(target, pattern)
        return [path] if file.exists(path) else []

    parts = pattern.split("/")
    filename = parts[-1]
    search_root = target
    for part in parts:
        if "*" in part:
            break
        search_root = file.join(search_root, part)

    if not file.exists(search_root) or not file.is_directory(search_root):
        return []

    files = []
    def collect(entry):
        if not entry.is_dir and entry.name == filename:
            files.append(file.join(search_root, entry.path))
    file.walk_tree(root=search_root, callback=collect)
    return files

def validate_file(file_path, schema_json):
    """Validate a single file against a schema."""
    content = file.read(file_path)
    data = yaml.decode(content)
    if data == None:
        return False, ["Failed to parse YAML"]

    valid, errors = schema.validate(data, schema_json)
    return valid, errors


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

    total_files = 0
    total_errors = 0
    validated_types = []

    for schema_type, config in SCHEMAS.items():
        schema_path = file.join(target, config["schema"])
        if not file.exists(schema_path):
            continue

        schema_json = file.read(schema_path)
        files = find_files(target, config["pattern"])

        if len(files) == 0:
            continue

        validated_types.append(schema_type)
        ui.note("Validating " + schema_type + " (" + str(len(files)) + " files)")

        for file_path in files:
            total_files = total_files + 1
            rel_path = file_path
            if file_path.startswith(target + "/"):
                rel_path = file_path[len(target) + 1:]

            valid, errors = validate_file(file_path, schema_json)
            if valid:
                ui.note("  " + rel_path)
            else:
                total_errors = total_errors + 1
                ui.error("  " + rel_path)
                for err in errors:
                    ui.error("    " + err)

    if len(validated_types) == 0:
        ui.warn("No knowledge schemas found")
        return

    if total_errors > 0:
        fail(str(total_errors) + " of " + str(total_files) + " files failed validation")
    else:
        ui.success("Validated " + str(total_files) + " files across " + str(len(validated_types)) + " types")
