# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# validate.star - Validate YAML files against JSON schemas
#
# This operation validates YAML files in devlore-registry against their
# corresponding JSON schemas.
#
# Usage:
#   star registry validate                           # all types
#   star registry validate --type=package            # all package.* types
#   star registry validate --type=package.lifecycle  # specific type
#   star registry validate --type=knowledge          # all knowledge.* types

# Schema definitions: type -> (file_pattern, schema_path)
# file_pattern uses {name} as placeholder for glob matching
SCHEMAS = {
    "package.lifecycle": {
        "pattern": "packages/*/lifecycle.yaml",
        "schema": "schemas/package.lifecycle.json",
    },
    "package.index": {
        "pattern": "packages/INDEX.yaml",
        "schema": "schemas/package.index.json",
    },
    "package.signatures": {
        "pattern": "signatures.yaml",
        "schema": "schemas/package.signatures.json",
    },
    "knowledge.index": {
        "pattern": "knowledge/*/index.yaml",
        "schema": "schemas/knowledge.index.json",
    },
}

def matches_type_filter(schema_type, type_filter):
    """Check if schema_type matches the type filter."""
    if type_filter == "":
        return True
    if schema_type == type_filter:
        return True
    # Check prefix match (e.g., "package" matches "package.lifecycle")
    if schema_type.startswith(type_filter + "."):
        return True
    return False

def find_files(registry, pattern):
    """Find files matching a glob-like pattern."""
    files = []

    # Handle patterns with wildcards
    if "*" in pattern:
        parts = pattern.split("/")
        base_path = registry

        # Find the directory containing the wildcard
        for i, part in enumerate(parts):
            if "*" in part:
                # Scan this directory
                remaining = "/".join(parts[i+1:])
                if file.exists(base_path) and file.is_directory(base_path):
                    for entry in file.list(base_path):
                        if entry.is_dir and part == "*":
                            candidate = file.join(entry.path, remaining)
                            if file.exists(candidate):
                                files.append(candidate)
                break
            else:
                base_path = file.join(base_path, part)
    else:
        # No wildcard - direct file
        path = file.join(registry, pattern)
        if file.exists(path):
            files.append(path)

    return files

def validate_file(file_path, schema_json):
    """Validate a single file against a schema."""
    content = file.read(file_path)
    data = yaml.decode(content)
    if data == None:
        return False, ["Failed to parse YAML"]

    valid, errors = schema.validate(data, schema_json)
    return valid, errors

def run(ctx):
    """Main entry point."""
    registry = ctx.args.get("path", ".")
    type_filter = ctx.args.get("type", "")

    total_files = 0
    total_errors = 0
    validated_types = []

    for schema_type, config in SCHEMAS.items():
        if not matches_type_filter(schema_type, type_filter):
            continue

        schema_path = file.join(registry, config["schema"])
        if not file.exists(schema_path):
            # Schema doesn't exist yet - skip silently
            continue

        schema_json = file.read(schema_path)
        files = find_files(registry, config["pattern"])

        if len(files) == 0:
            continue

        validated_types.append(schema_type)
        note("Validating " + schema_type + " (" + str(len(files)) + " files)")

        for file_path in files:
            total_files = total_files + 1
            rel_path = file_path
            if file_path.startswith(registry + "/"):
                rel_path = file_path[len(registry) + 1:]

            valid, errors = validate_file(file_path, schema_json)
            if valid:
                note("  " + rel_path)
            else:
                total_errors = total_errors + 1
                error("  " + rel_path)
                for err in errors:
                    error("    " + err)

    if len(validated_types) == 0:
        if type_filter != "":
            fail("No schemas found for type: " + type_filter)
        else:
            warn("No schemas found")
        return

    if total_errors > 0:
        fail(str(total_errors) + " of " + str(total_files) + " files failed validation")
    else:
        success("Validated " + str(total_files) + " files across " + str(len(validated_types)) + " types")