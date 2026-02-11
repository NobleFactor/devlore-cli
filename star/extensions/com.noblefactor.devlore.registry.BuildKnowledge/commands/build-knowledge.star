# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

#
# build-knowledge.star - Build knowledge artifacts from devlore-cli source
#
# This is a build step that:
# 1. Interrogates devlore-cli source code (static analysis via Go AST)
# 2. Enforces contracts (fails build on violations)
# 3. Rebuilds knowledge artifacts in devlore-registry
#
# Domains:
#   - onboarding: Starlark API reference for lore package authors
#   - migration: writ migrate patterns (source systems, encryption, execution ops)
#   - all: Both domains (default)

def run(ctx):
    """Main entry point for build-knowledge command."""
    source_path = ctx.args.get("source_path", "")
    registry_path = ctx.args.get("registry_path", "")
    domain = ctx.args.get("domain", "all")

    # Smart defaults: look for sibling directories
    if not source_path:
        source_path = _find_sibling("devlore-cli")
        if source_path:
            note("Using sibling source: " + source_path)
        else:
            fail("--source-path required (no ../devlore-cli found)")

    if not registry_path:
        registry_path = _find_sibling("devlore-registry")
        if registry_path:
            note("Using sibling registry: " + registry_path)
        else:
            fail("--registry-path required (no ../devlore-registry found)")

    # Validate paths exist
    if not file.is_directory(source_path):
        fail("Source path not found: " + source_path)
    if not file.is_directory(registry_path):
        fail("Registry path not found: " + registry_path)

    # Build knowledge for selected domain(s)
    if domain == "all" or domain == "onboarding":
        build_onboarding_knowledge(source_path, registry_path)

    if domain == "all" or domain == "migration":
        build_migration_knowledge(source_path, registry_path)


def _find_sibling(name):
    """Find a sibling directory by name."""
    # Try ../name relative to current directory
    sibling = file.join("..", name)
    if file.is_directory(sibling):
        return sibling
    return ""


# =============================================================================
# ONBOARDING KNOWLEDGE (Starlark API reference for lore package authors)
# =============================================================================

def build_onboarding_knowledge(source_path, registry_path):
    """Build Starlark API reference from devlore-cli source.

    Uses go.parse_devlore_api() to extract the API from Go source code.
    Writes the hierarchical API reference to the registry.
    """
    note("Building onboarding knowledge (Starlark API)...")

    starlark_path = file.join(source_path, "internal", "starlark")
    if not file.is_directory(starlark_path):
        fail("Starlark package not found: " + starlark_path)

    # Parse the API using Go AST - returns hierarchical structure
    note("  Scanning " + starlark_path + "...")
    api = go.parse_devlore_api(starlark_path)

    # Count bindings
    binding_count = _count_bindings(api)
    violation_count = len(list(api.violations))

    note("  Found " + str(binding_count) + " bindings")

    # Check for contract violations
    if violation_count > 0:
        error("Contract violations detected:")
        for v in api.violations:
            error("  " + v.name + " (" + v.file + ":" + str(v.line) + "): " + v.error)
        fail("Fix contract violations before building knowledge")

    success("  No contract violations")

    # Convert to dict for YAML serialization
    api_dict = _api_to_dict(api)

    # Write to registry
    reference_path = file.join(registry_path, "knowledge", "package-authoring", "bindings", "reference.yaml")

    # Compare with existing
    changes_detected = False
    new_content = yaml.encode(api_dict)
    if file.exists(reference_path):
        current_content = file.read(reference_path)
        if current_content != new_content:
            changes_detected = True
            note("  Changes detected in reference.yaml")
    else:
        changes_detected = True
        note("  Creating new reference.yaml")

    if changes_detected:
        file.write(reference_path, new_content)
        success("  Wrote " + reference_path)
    else:
        success("  No changes to reference.yaml")


def _count_bindings(api):
    """Count total bindings in the hierarchical API."""
    count = 0
    for ns in dir(api.plan):
        if not ns.startswith("_"):
            count += len(list(getattr(api.plan, ns)))
    for ns in dir(api.system):
        if not ns.startswith("_"):
            count += len(list(getattr(api.system, ns)))
    return count


def _api_to_dict(api):
    """Convert API struct to dict for YAML serialization."""
    result = {
        "valid": bool(api.valid),
        "plan": {},
        "system": {},
        "violations": [],
    }

    # Convert plan namespaces
    for ns in dir(api.plan):
        if ns.startswith("_"):
            continue
        methods = list(getattr(api.plan, ns))
        result["plan"][ns] = [_method_to_dict(m) for m in methods]

    # Convert system namespaces
    for ns in dir(api.system):
        if ns.startswith("_"):
            continue
        methods = list(getattr(api.system, ns))
        result["system"][ns] = [_method_to_dict(m) for m in methods]

    # Convert violations
    for v in api.violations:
        result["violations"].append({
            "name": v.name,
            "file": v.file,
            "line": int(v.line),
            "error": v.error,
        })

    return result


def _method_to_dict(m):
    """Convert a method struct to dict."""
    # Convert slot_docs struct to dict
    slot_docs = {}
    for slot in m.slots:
        doc = getattr(m.slot_docs, slot, "")
        if doc:
            slot_docs[slot] = doc

    return {
        "name": m.name,
        "full_name": m.full_name,
        "doc": m.doc,
        "usage": m.usage,
        "slots": list(m.slots),
        "slot_docs": slot_docs,
        "operations": list(m.operations),
        "output": m.output,
        "returns": m.returns,
        "file": m.file,
        "line": int(m.line),
    }


# =============================================================================
# MIGRATION KNOWLEDGE (Writ migrate patterns for writ migrate)
# =============================================================================

def build_migration_knowledge(source_path, registry_path):
    """Build migration knowledge from writ migrate source.

    This validates that the Go source constants match the registry knowledge:
    - SourceSystem constants should have corresponding signature files
    - EncryptionSystem constants should be documented
    - Platform names should match writ-structure.yaml
    - Execution operations in schemas match ops.go
    """
    note("Building migration knowledge...")

    migrate_path = file.join(source_path, "internal", "writ", "migrate")
    if not file.is_directory(migrate_path):
        fail("Migrate source not found: " + migrate_path)

    execution_path = file.join(source_path, "internal", "execution")
    if not file.is_directory(execution_path):
        fail("Execution source not found: " + execution_path)

    knowledge_path = file.join(registry_path, "knowledge", "migration")
    if not file.is_directory(knowledge_path):
        fail("Migration knowledge path not found: " + knowledge_path)

    # Step 1: Parse Go source files
    note("  Scanning " + migrate_path + "...")
    result = go.parse_migrate_knowledge(migrate_path)

    source_systems = list(result.source_systems)
    encryption_systems = list(result.encryption_systems)
    repo_layers = list(result.repo_layers)
    platforms = list(result.platforms)

    note("  Found " + str(len(source_systems)) + " source systems")
    note("  Found " + str(len(encryption_systems)) + " encryption systems")
    note("  Found " + str(len(platforms)) + " platforms")

    # Step 1b: Parse execution operations from ops.go
    ops_path = file.join(execution_path, "ops.go")
    note("  Scanning " + ops_path + "...")
    ops_result = go.parse_execution_ops(ops_path)
    execution_ops = list(ops_result.operations)
    note("  Found " + str(len(execution_ops)) + " execution operations")

    # Step 2: Load registry signature files
    # Only include files that look like actual system signatures (have a 'name' field)
    signatures_path = file.join(knowledge_path, "signatures")
    registry_systems = []
    if file.is_directory(signatures_path):
        for entry in file.list(signatures_path):
            if entry.name.endswith(".yaml"):
                sig_path = file.join(signatures_path, entry.name)
                content = file.read(sig_path)
                sig = yaml.decode(content)
                # Only consider files with a 'name' field as system signatures
                if sig.get("name"):
                    system_name = entry.name.replace(".yaml", "")
                    registry_systems.append(system_name)

    # Step 3: Load writ-structure.yaml for platform validation
    writ_structure_path = file.join(knowledge_path, "concepts", "writ-structure.yaml")
    registry_platforms = []
    registry_platform_aliases = []
    if file.exists(writ_structure_path):
        content = file.read(writ_structure_path)
        structure = yaml.decode(content)
        segments = structure.get("naming", {}).get("segments", {})
        platform_list = segments.get("platforms", [])
        for p in platform_list:
            if "name" in p:
                registry_platforms.append(p["name"])
            # Also collect aliases
            aliases = p.get("aliases", [])
            for alias in aliases:
                registry_platform_aliases.append(alias)

    # Step 4: Check for contract violations (source vs registry consistency)
    violations = check_migration_contract_violations(
        source_systems,
        encryption_systems,
        platforms,
        registry_systems,
        registry_platforms,
        registry_platform_aliases,
    )

    if violations:
        error("Contract violations detected:")
        for v in violations:
            error("  " + v["type"] + ": " + v["message"])
        fail("Fix contract violations before building knowledge")

    success("  No contract violations")

    # Step 5: Generate/update systems reference file
    systems_ref = generate_systems_reference(source_systems, encryption_systems, repo_layers, platforms)
    systems_ref_path = file.join(knowledge_path, "systems-reference.yaml")

    # Compare with existing
    changes_detected = False
    if file.exists(systems_ref_path):
        current_content = file.read(systems_ref_path)
        new_content = yaml.encode(systems_ref)
        if current_content != new_content:
            changes_detected = True
            note("  Changes detected in systems-reference.yaml")
    else:
        changes_detected = True
        note("  Creating new systems-reference.yaml")

    if changes_detected:
        file.write(systems_ref_path, yaml.encode(systems_ref))
        success("  Wrote " + systems_ref_path)
    else:
        success("  No changes to systems-reference.yaml")

    # Step 6: Validate all signature files exist for source systems
    validate_signature_coverage(source_systems, signatures_path)

    # Step 7: Generate execution graph schema from Go types
    generate_execution_schema(source_path, knowledge_path)


def generate_execution_schema(source_path, knowledge_path):
    """Generate engine-graph.json schema from Go struct definitions.

    This ensures the schema is always derived from the actual Go types.
    """
    schemas_path = file.join(knowledge_path, "schemas")
    if not file.is_directory(schemas_path):
        warn("  Schemas path not found: " + schemas_path)
        return

    # Parse Go execution package for structs and operations
    execution_path = file.join(source_path, "internal", "execution")
    note("  Generating schema from " + execution_path + "...")
    schema_data = go.parse_execution_schema(execution_path)

    # Build JSON Schema from Go types
    engine_schema = _build_engine_graph_schema(schema_data)

    # Write schema
    engine_schema_path = file.join(schemas_path, "engine-graph.json")
    new_content = json.encode_indent(engine_schema, "  ")

    changes_detected = False
    if file.exists(engine_schema_path):
        current_content = file.read(engine_schema_path)
        if current_content != new_content:
            changes_detected = True
            note("  Changes detected in engine-graph.json")
    else:
        changes_detected = True
        note("  Creating new engine-graph.json")

    if changes_detected:
        file.write(engine_schema_path, new_content)
        success("  Wrote " + engine_schema_path)
    else:
        success("  No changes to engine-graph.json")


def _build_engine_graph_schema(schema_data):
    """Build JSON Schema from parsed Go struct data.

    Generates a complete JSON Schema for execution graphs based on the
    Go struct definitions in the execution package.
    """
    ops_list = sorted(list(schema_data.operations))

    # Build SlotValue schema from Go struct
    slot_value_schema = {
        "type": "object",
        "description": "A slot value - either immediate or a promise (reference to another node)",
        "properties": {},
    }
    if hasattr(schema_data, "slot_value"):
        for f in schema_data.slot_value.fields:
            slot_value_schema["properties"][f.json_name] = _field_to_json_schema(f)

    # Build Node schema from Go struct
    node_properties = {}
    node_required = []
    if hasattr(schema_data, "node"):
        for f in schema_data.node.fields:
            # Special handling for operations - add enum
            if f.json_name == "operations":
                node_properties["operations"] = {
                    "type": "array",
                    "description": "Pipeline of operations to execute",
                    "items": {
                        "type": "string",
                        "enum": ops_list,
                    },
                }
            # Special handling for slots - reference slot_value
            elif f.json_name == "slots":
                node_properties["slots"] = {
                    "type": "object",
                    "description": "Input slots for this node (name -> value)",
                    "additionalProperties": {"$ref": "#/$defs/slot_value"},
                }
            # Special handling for status - add enum
            elif f.json_name == "status":
                statuses = list(schema_data.node_statuss) if hasattr(schema_data, "node_statuss") else ["pending", "completed", "skipped", "failed"]
                node_properties["status"] = {
                    "type": "string",
                    "description": f.description if f.description else "Execution status of this node",
                    "enum": statuses,
                }
            else:
                node_properties[f.json_name] = _field_to_json_schema(f)

            if f.required and f.json_name not in ["status"]:  # status has default
                node_required.append(f.json_name)

    # Build Edge schema from Go struct
    edge_properties = {}
    edge_required = []
    if hasattr(schema_data, "edge"):
        for f in schema_data.edge.fields:
            edge_properties[f.json_name] = _field_to_json_schema(f)
            if f.required:
                edge_required.append(f.json_name)

    # Build complete schema
    schema = {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://devlore.noblefactor.com/schemas/engine-graph.json",
        "title": "Execution Graph",
        "description": "Execution graph derived from devlore-cli execution.Graph Go types",
        "type": "object",
        "$defs": {
            "slot_value": slot_value_schema,
            "node": {
                "type": "object",
                "properties": node_properties,
                "required": node_required if node_required else ["id", "operations"],
            },
            "edge": {
                "type": "object",
                "properties": edge_properties,
                "required": edge_required if edge_required else ["from", "to", "relation"],
            },
        },
        "properties": {
            "version": {
                "type": "string",
                "description": "Graph format version",
            },
            "tool": {
                "type": "string",
                "description": "Tool that created this graph",
            },
            "state": {
                "type": "string",
                "description": "Execution state of the graph",
                "enum": list(schema_data.graph_states) if hasattr(schema_data, "graph_states") else ["pending", "executed", "failed"],
            },
            "nodes": {
                "type": "array",
                "description": "Operations to execute",
                "items": {"$ref": "#/$defs/node"},
            },
            "edges": {
                "type": "array",
                "description": "Dependencies between nodes",
                "items": {"$ref": "#/$defs/edge"},
            },
        },
        "required": ["nodes"],
    }

    return schema


def _field_to_json_schema(field):
    """Convert a Go struct field to JSON Schema property."""
    go_type = field.type
    schema = {}

    if field.description:
        schema["description"] = field.description

    # Map Go types to JSON Schema types
    if go_type == "string":
        schema["type"] = "string"
    elif go_type == "int" or go_type == "int64":
        schema["type"] = "integer"
    elif go_type == "bool":
        schema["type"] = "boolean"
    elif go_type == "float64":
        schema["type"] = "number"
    elif go_type.startswith("[]"):
        schema["type"] = "array"
        inner_type = go_type[2:]
        if inner_type == "string":
            schema["items"] = {"type": "string"}
        else:
            schema["items"] = {"type": "object"}
    elif go_type.startswith("map["):
        schema["type"] = "object"
        schema["additionalProperties"] = {"type": "string"}
    elif go_type == "os.FileMode":
        schema["type"] = "integer"
        schema["description"] = (field.description + " " if field.description else "") + "(octal file permissions)"
    elif go_type == "time.Time":
        schema["type"] = "string"
        schema["format"] = "date-time"
    else:
        # Custom type - treat as string for now
        schema["type"] = "string"

    return schema


def check_migration_contract_violations(source_systems, encryption_systems, platforms, registry_systems, registry_platforms, registry_platform_aliases):
    """Check for contract violations between source code and registry.

    Contract:
      - Every SourceSystem constant (except 'unknown') should have a signature file
      - Registry platforms should match source platforms (including aliases)
    """
    violations = []

    # Get source system values (excluding unknown and native which don't need signatures)
    source_system_values = []
    for s in source_systems:
        val = str(s.value)
        if val not in ["unknown", "native"]:
            source_system_values.append(val)

    # Check: source systems should have registry signatures
    for system in source_system_values:
        if system not in registry_systems:
            violations.append({
                "type": "missing_signature",
                "message": "Source system '" + system + "' has no signature file in registry",
            })

    # Check: registry signatures should have source system constants
    for system in registry_systems:
        # Skip encryption systems (git-crypt is an encryption system, not a source system)
        if system in ["git-crypt", "sops", "age", "gpg", "blackbox", "transcrypt", "ansible-vault"]:
            continue
        found = False
        for s in source_systems:
            if str(s.value) == system:
                found = True
                break
        if not found:
            violations.append({
                "type": "orphan_signature",
                "message": "Registry signature '" + system + "' has no SourceSystem constant",
            })

    # Check: platforms in prompt should match registry platforms (or be an alias)
    # Aliases are case-insensitive (ubuntu is alias for Debian)
    if registry_platforms:
        all_valid_platforms = set(registry_platforms)
        for alias in registry_platform_aliases:
            all_valid_platforms.add(alias)
            all_valid_platforms.add(alias.lower())
            all_valid_platforms.add(alias.title())

        for platform in platforms:
            if platform not in all_valid_platforms and platform.lower() not in all_valid_platforms:
                violations.append({
                    "type": "undocumented_platform",
                    "message": "Platform '" + platform + "' in LLM prompt not in writ-structure.yaml",
                })

    return violations


def generate_systems_reference(source_systems, encryption_systems, repo_layers, platforms):
    """Generate systems-reference.yaml from Go source constants."""
    ref = {
        "version": "1.0",
        "source": "devlore-cli/internal/writ/migrate",
        "generated": True,
        "description": "Auto-generated reference of migrate constants from Go source",
    }

    # Source systems
    systems = []
    for s in source_systems:
        systems.append({
            "name": str(s.name),
            "value": str(s.value),
            "file": str(s.file),
            "line": int(s.line),
        })
    ref["source_systems"] = systems

    # Encryption systems
    encryptions = []
    for e in encryption_systems:
        encryptions.append({
            "name": str(e.name),
            "value": str(e.value),
            "file": str(e.file),
            "line": int(e.line),
        })
    ref["encryption_systems"] = encryptions

    # Repo layers
    layers = []
    for r in repo_layers:
        layers.append({
            "name": str(r.name),
            "value": str(r.value),
            "file": str(r.file),
            "line": int(r.line),
        })
    ref["repo_layers"] = layers

    # Platforms from LLM prompt
    ref["platforms"] = [str(p) for p in platforms]

    return ref


def validate_signature_coverage(source_systems, signatures_path):
    """Validate that all source systems have proper signature files."""
    for s in source_systems:
        val = str(s.value)
        if val in ["unknown", "native"]:
            continue

        sig_file = file.join(signatures_path, val + ".yaml")
        if not file.exists(sig_file):
            warn("  Missing signature file: " + sig_file)
        else:
            # Validate signature file has required fields
            content = file.read(sig_file)
            sig = yaml.decode(content)
            if not sig.get("name"):
                warn("  Signature missing 'name': " + sig_file)
            if not sig.get("markers"):
                warn("  Signature missing 'markers': " + sig_file)