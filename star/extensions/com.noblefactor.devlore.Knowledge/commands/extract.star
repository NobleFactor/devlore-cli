# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

#
# extract.star - Extract knowledge artifacts from devlore-cli source
#
# This is a build step that:
# 1. Interrogates devlore-cli source code (static analysis via Go AST)
# 2. Enforces contracts (fails build on violations)
# 3. Writes knowledge artifacts to --target
#
# Domains:
#   - onboarding: Starlark API reference for lore package authors
#   - migration: writ migrate patterns (source systems, encryption, execution ops)
#   - ops: operation surface mappings from *Service structs
#   - all: All domains (default)
#
# Usage:
#   star devlore knowledge extract --source=. --target=/tmp/out
#   star devlore knowledge extract --domain=migration

# Source file conventions for migration knowledge
_ANALYSIS_FILE = "analysis.go"
_PLAN_FILE = "plan.go"
_GRAPH_FILE = "graph.go"

# Known properties — non-callable attributes that can't be detected by call scanning.
# These appear as direct value returns in Attr() switch cases or FromStringDict struct fields.
_KNOWN_PROPERTIES = {
    "package.name":             {"type": "string", "doc": "Package name being deployed"},
    "package.version":          {"type": "string", "doc": "Version being deployed"},
    "package.features":         {"type": "list[string]", "doc": "Enabled features"},
    "package.settings":         {"type": "dict[string, string]", "doc": "Key-value settings"},
    "package.dry_run":          {"type": "bool", "doc": "True if this is a preview run"},
    "package.source_root":      {"type": "string", "doc": "Package source directory"},
    "package.target_root":      {"type": "string", "doc": "Deployment target directory"},
    "system.platform.os":       {"type": "string", "doc": "Operating system (darwin, linux, windows)"},
    "system.platform.arch":     {"type": "string", "doc": "Architecture (amd64, arm64)"},
    "system.platform.distro":   {"type": "string", "doc": "Distribution codename"},
    "system.platform.version":  {"type": "string", "doc": "OS version string"},
    "system.platform.hostname": {"type": "string", "doc": "Machine hostname"},
}

# AttrNames declarations from Go source — authoritative cross-reference.
# If a name appears here but isn't found as a binding, property, or sub-namespace,
# the extract pipeline fails. Update this when adding new attrs in Go.
_ATTR_NAMES = {
    "plan": ["archive", "download", "encryption", "file", "gather", "git", "literal", "package", "service", "shell", "source", "template"],
    "plan.file": ["copy", "link", "remove", "write"],
    "plan.package": ["install", "remove", "update", "upgrade"],
    "plan.template": ["render"],
    "plan.encryption": ["decrypt"],
    "plan.archive": ["extract"],
    "plan.git": ["checkout", "clone", "pull"],
    "system": ["file", "git", "package", "platform", "service"],
    "system.package": ["installed", "manager", "version"],
    "system.service": ["enabled", "exists", "running"],
    "system.git": ["current_branch", "installed", "is_clean", "repo_root", "version"],
    "system.file": ["exists", "home", "is_dir", "which"],
    "package": ["dry_run", "features", "has_feature", "name", "setting", "settings", "source_root", "target_root", "version"],
    "phase": ["retry"],
}


def run(ctx):
    """Main entry point for extract command."""
    source = ctx.args.get("source", "")
    target = _resolve_target(ctx)
    domain = ctx.args.get("domain", "all")

    # Smart default for source: look for sibling directory
    if not source:
        source = _find_sibling("devlore-cli")
        if source:
            note("Using sibling source: " + source)
        else:
            fail("--source required (no ../devlore-cli found)")

    # Validate paths exist
    if not file.is_directory(source):
        fail("Source path not found: " + source)

    # Build knowledge for selected domain(s)
    if domain == "all" or domain == "onboarding":
        build_onboarding_knowledge(source, target)

    if domain == "all" or domain == "migration":
        build_migration_knowledge(source, target)

    if domain == "all" or domain == "ops":
        build_ops_knowledge(source, target)


def _resolve_target(ctx):
    """Resolve --target flag or auto-detect sibling devlore-registry."""
    target = ctx.args.get("target", "")
    if not target:
        target = _find_sibling("devlore-registry")
        if target:
            note("Using sibling registry: " + target)
        else:
            fail("--target required (no ../devlore-registry found)")
    if not file.is_directory(target):
        fail("Target path not found: " + target)
    return target


def _find_sibling(name):
    """Find a sibling directory by name."""
    sibling = file.join("..", name)
    if file.is_directory(sibling):
        return sibling
    return ""


# =============================================================================
# GO AST HELPERS
# =============================================================================

def _parse_devlore_api(path):
    """Parse devlore-cli Starlark API from Go source files.

    Uses go.methods() and go.calls() to extract:
    - Binding names from Attr methods (via NewBuiltin and MakeAttr calls)
    - Handler details: doc, slots, operations, output
    - Properties from _KNOWN_PROPERTIES
    - StringDict violations (non-Attr NewBuiltin registrations)
    - AttrNames cross-reference violations
    """
    # Step 1: Find all Attr methods and extract bindings via NewBuiltin
    attr_methods = go.methods(path, name="Attr")
    bindings = {}
    seen = {}

    for attr_method in attr_methods:
        builtin_calls = go.calls(attr_method.scope, name="NewBuiltin")
        for call in builtin_calls:
            args = list(call.args)
            if len(args) < 2:
                continue
            binding_name = str(args[0].string_value)
            handler_name = str(args[1].ident_name)
            if not binding_name or not _is_api_binding(binding_name):
                continue
            if binding_name in seen:
                continue
            seen[binding_name] = True
            binding = _extract_binding_info(path, handler_name, binding_name, str(attr_method.file), int(call.line))
            if binding:
                binding["kind"] = "method"
                bindings[binding_name] = binding

    # Step 1b: Extract bindings via MakeAttr (same 2-arg signature as NewBuiltin)
    for attr_method in attr_methods:
        make_attr_calls = go.calls(attr_method.scope, name="MakeAttr")
        for call in make_attr_calls:
            args = list(call.args)
            if len(args) < 2:
                continue
            binding_name = str(args[0].string_value)
            handler_name = str(args[1].ident_name)
            if not binding_name or not _is_api_binding(binding_name):
                continue
            if binding_name in seen:
                continue
            seen[binding_name] = True
            binding = _extract_binding_info(path, handler_name, binding_name, str(attr_method.file), int(call.line))
            if binding:
                binding["kind"] = "method"
                bindings[binding_name] = binding

    # Step 2: Inject known properties
    for prop_name, prop_info in _KNOWN_PROPERTIES.items():
        if prop_name in seen:
            continue
        seen[prop_name] = True
        bindings[prop_name] = {
            "kind": "property",
            "value_type": prop_info["type"],
            "doc": prop_info["doc"],
            "usage": "",
            "slots": [],
            "slot_docs": {},
            "operations": [],
            "output": "none",
            "returns": "",
            "file": "",
            "line": 0,
        }

    # Step 3: Detect StringDict violations
    # Any NewBuiltin call for API bindings outside Attr methods is a violation
    all_methods = go.methods(path)
    violations = []
    for method in all_methods:
        if str(method.name) == "Attr":
            continue
        nb_calls = go.calls(method.scope, name="NewBuiltin")
        for call in nb_calls:
            args = list(call.args)
            if len(args) < 1:
                continue
            binding_name = str(args[0].string_value)
            if not binding_name or not _is_api_binding(binding_name):
                continue
            if binding_name in seen:
                continue
            seen[binding_name] = True
            violations.append({
                "name": binding_name,
                "file": str(method.file),
                "line": int(call.line),
                "error": "uses StringDict instead of Attr receiver",
            })

    # Step 4: AttrNames cross-reference validation
    attr_violations = _validate_attr_names(bindings)
    violations.extend(attr_violations)

    # Step 5: Build hierarchical result
    plan = {}
    system = {}
    package = {}
    phase = {}
    for name in sorted(bindings.keys()):
        b = bindings[name]
        parts = name.split(".")
        if len(parts) < 2:
            continue
        context = parts[0]
        if len(parts) == 2:
            namespace = "(root)"
            method_name = parts[1]
        else:
            namespace = parts[1]
            method_name = parts[-1]
        entry = {
            "name": method_name,
            "full_name": name,
            "kind": b.get("kind", "method"),
            "doc": b.get("doc", ""),
            "usage": b.get("usage", ""),
            "slots": b.get("slots", []),
            "slot_docs": b.get("slot_docs", {}),
            "operations": b.get("operations", []),
            "output": b.get("output", "none"),
            "returns": b.get("returns", ""),
            "file": b.get("file", ""),
            "line": b.get("line", 0),
        }
        if b.get("kind") == "property":
            entry["value_type"] = b.get("value_type", "")
        if context == "plan":
            if namespace not in plan:
                plan[namespace] = []
            plan[namespace].append(entry)
        elif context == "system":
            if namespace not in system:
                system[namespace] = []
            system[namespace].append(entry)
        elif context == "package":
            if namespace not in package:
                package[namespace] = []
            package[namespace].append(entry)
        elif context == "phase":
            if namespace not in phase:
                phase[namespace] = []
            phase[namespace].append(entry)

    return {
        "valid": len(violations) == 0,
        "plan": plan,
        "system": system,
        "package": package,
        "phase": phase,
        "violations": violations,
    }


def _is_api_binding(name):
    """Check if a binding name is a lore API binding."""
    return (name.startswith("plan.") or name.startswith("system.") or
            name.startswith("package.") or name.startswith("phase."))


def _validate_attr_names(bindings):
    """Cross-reference AttrNames declarations against collected bindings.

    Every name declared in _ATTR_NAMES must be accounted for as either:
    - A binding (found via NewBuiltin/MakeAttr scanning)
    - A property (in _KNOWN_PROPERTIES)
    - A sub-namespace reference (has child bindings like prefix.name.*)
    """
    violations = []
    all_binding_names = set(bindings.keys())

    for prefix, names in _ATTR_NAMES.items():
        for name in names:
            qualified = prefix + "." + name
            # Check if it's a direct binding or property
            if qualified in all_binding_names:
                continue
            # Check if it's a sub-namespace (has children like prefix.name.*)
            child_prefix = qualified + "."
            has_children = False
            for b in all_binding_names:
                if b.startswith(child_prefix):
                    has_children = True
                    break
            if has_children:
                continue
            violations.append({
                "name": qualified,
                "file": "",
                "line": 0,
                "error": "declared in AttrNames but not found as binding, property, or sub-namespace",
            })

    return violations


def _extract_binding_info(path, handler_name, binding_name, fallback_file, fallback_line):
    """Extract binding info from a handler method."""
    if not handler_name:
        return {"file": fallback_file, "line": fallback_line}

    # Find the handler method
    handler_methods = go.methods(path, name=handler_name)
    if len(list(handler_methods)) == 0:
        return {"file": fallback_file, "line": fallback_line}

    handler = list(handler_methods)[0]
    scope = str(handler.scope)

    # Parse doc comment
    doc, usage, slot_docs, returns = _parse_doc_comment(str(handler.doc))

    # Extract slots from FillSlot calls
    slots = []
    slot_seen = {}
    fill_calls = go.calls(scope, name="FillSlot")
    for call in fill_calls:
        args = list(call.args)
        if len(args) >= 3:
            slot_name = str(args[2].string_value)
            if slot_name and slot_name not in slot_seen:
                slot_seen[slot_name] = True
                slots.append(slot_name)

    # Extract operations from execution.Node composites
    operations = []
    node_composites = go.composites(scope, type="execution.Node")
    for comp in node_composites:
        ops_field = getattr(comp.fields, "Operations", None)
        if ops_field:
            for op in ops_field:
                op_str = str(op)
                if op_str not in operations:
                    operations.append(op_str)

    # Detect output type (promise if NewOutput is called)
    output = "none"
    output_calls = go.calls(scope, name="NewOutput")
    if len(list(output_calls)) > 0:
        output = "promise"

    return {
        "doc": doc,
        "usage": usage,
        "slots": slots,
        "slot_docs": slot_docs,
        "operations": operations,
        "output": output,
        "returns": returns,
        "file": str(handler.file),
        "line": int(handler.line),
    }


def _parse_doc_comment(doc_text):
    """Parse structured doc comment into components."""
    description = ""
    usage = ""
    slot_docs = {}
    returns = ""

    if not doc_text:
        return description, usage, slot_docs, returns

    lines = doc_text.split("\n")
    desc_lines = []
    in_slots = False

    for line in lines:
        line = line.strip()
        if line.startswith("Usage:"):
            usage = line[len("Usage:"):].strip()
            in_slots = False
        elif line.startswith("Slots:"):
            in_slots = True
        elif line.startswith("Returns:"):
            returns = line[len("Returns:"):].strip()
            in_slots = False
        elif in_slots and line.startswith("- "):
            slot_line = line[2:]
            colon_idx = slot_line.find(":")
            if colon_idx > 0:
                slot_docs[slot_line[:colon_idx].strip()] = slot_line[colon_idx + 1:].strip()
        elif in_slots and line == "":
            in_slots = False
        elif not usage and not in_slots and not returns and line:
            desc_lines.append(line)

    description = " ".join(desc_lines)
    return description, usage, slot_docs, returns


def _parse_migrate_knowledge(path):
    """Parse migration constants from Go source using go.const_groups and go.raw_string."""
    analysis_path = file.join(path, _ANALYSIS_FILE)
    plan_path = file.join(path, _PLAN_FILE)

    source_systems = []
    encryption_systems = []
    repo_layers = []

    # Parse typed const groups from analysis.go
    if file.exists(analysis_path):
        groups = go.const_groups(analysis_path)
        for group in groups:
            type_name = str(group.type_name)
            for c in group.constants:
                entry = {
                    "name": str(c.name),
                    "value": str(c.value),
                    "type_name": type_name,
                    "file": str(group.file),
                    "line": int(c.line),
                }
                if type_name == "SourceSystem":
                    source_systems.append(entry)
                elif type_name == "EncryptionSystem":
                    encryption_systems.append(entry)
                elif type_name == "RepoLayer":
                    repo_layers.append(entry)

    # Extract system prompt and platforms from plan.go
    system_prompt = ""
    platforms = []
    if file.exists(plan_path):
        prompt_funcs = go.funcs(plan_path, name="buildSystemPrompt")
        func_list = list(prompt_funcs)
        if len(func_list) > 0:
            system_prompt = str(go.raw_string(func_list[0].scope))
            platforms = _extract_platforms(system_prompt)

    return {
        "source_systems": source_systems,
        "encryption_systems": encryption_systems,
        "repo_layers": repo_layers,
        "platforms": platforms,
        "system_prompt": system_prompt,
    }


def _extract_platforms(prompt_text):
    """Extract platform names from system prompt text."""
    platforms = []
    lines = prompt_text.split("\n")
    in_platform_section = False

    for i, line in enumerate(lines):
        if "Known platforms:" in line:
            colon_idx = line.find(":")
            if colon_idx >= 0:
                platform_str = line[colon_idx + 1:].strip()
                if platform_str:
                    for p in platform_str.split(","):
                        p = p.strip()
                        if p:
                            platforms.append(p)
                    break
            in_platform_section = True
            continue

        if in_platform_section:
            trimmed = line.strip()
            if trimmed.startswith("- "):
                entry = trimmed[2:]
                platform = entry
                space_idx = entry.find(" ")
                if space_idx > 0:
                    platform = entry[:space_idx]
                paren_idx = platform.find("(")
                if paren_idx > 0:
                    platform = platform[:paren_idx]
                platform = platform.strip()
                if platform:
                    platforms.append(platform)
            elif trimmed and not trimmed.startswith("-"):
                in_platform_section = False

    return platforms


def _scan_execution(path):
    """Scan execution package: structs, consts, and ops in one pass."""
    graph_path = file.join(path, _GRAPH_FILE)
    structs = go.structs(graph_path) if file.exists(graph_path) else []
    consts = go.const_groups(graph_path) if file.exists(graph_path) else []

    # Find Name() methods returning string on *Op types
    methods = go.methods(path, name="Name", returns="string")
    ops = []
    seen_ops = {}
    for m in methods:
        recv_type = str(m.receiver_type).lstrip("*")
        if recv_type.endswith("Op"):
            name = str(go.return_string(m.scope))
            if name and name not in seen_ops:
                seen_ops[name] = True
                ops.append({"name": name, "type": recv_type})
    ops = sorted(ops, key=lambda o: o["name"])

    return {"structs": list(structs), "consts": list(consts), "ops": ops}


# =============================================================================
# ONBOARDING KNOWLEDGE (Starlark API reference for lore package authors)
# =============================================================================

def build_onboarding_knowledge(source, target):
    """Build Starlark API reference from devlore-cli source."""
    note("Building onboarding knowledge (Starlark API)...")

    starlark_path = file.join(source, "internal", "starlark")
    if not file.is_directory(starlark_path):
        fail("Starlark package not found: " + starlark_path)

    # Parse the API using Go AST primitives
    note("  Scanning " + starlark_path + "...")
    api = _parse_devlore_api(starlark_path)

    # Count bindings
    binding_count = _count_bindings(api)
    violation_count = len(api["violations"])

    note("  Found " + str(binding_count) + " bindings")

    # Check for contract violations
    if violation_count > 0:
        error("Contract violations detected:")
        for v in api["violations"]:
            error("  " + v["name"] + " (" + v["file"] + ":" + str(v["line"]) + "): " + v["error"])
        fail("Fix contract violations before building knowledge")

    success("  No contract violations")

    # Write YAML reference
    reference_path = file.join(target, "knowledge", "package-authoring", "bindings", "reference.yaml")

    changes_detected = False
    new_content = yaml.encode(api)
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

    # Write markdown reference
    md_path = file.join(target, "knowledge", "package-authoring", "bindings", "reference.md")
    md_content = _render_markdown(api)

    md_changed = False
    if file.exists(md_path):
        current_md = file.read(md_path)
        if current_md != md_content:
            md_changed = True
            note("  Changes detected in reference.md")
    else:
        md_changed = True
        note("  Creating new reference.md")

    if md_changed:
        file.write(md_path, md_content)
        success("  Wrote " + md_path)
    else:
        success("  No changes to reference.md")


def _count_bindings(api):
    """Count total bindings in the API dict."""
    count = 0
    for ns in api["plan"]:
        count += len(api["plan"][ns])
    for ns in api["system"]:
        count += len(api["system"][ns])
    for ns in api["package"]:
        count += len(api["package"][ns])
    for ns in api["phase"]:
        count += len(api["phase"][ns])
    return count


# =============================================================================
# MARKDOWN RENDERER
# =============================================================================

def _render_markdown(api):
    """Render the API dict as markdown reference documentation."""
    lines = []

    # Header
    lines.append("<!-- Auto-generated by: star devlore knowledge extract --domain=onboarding -->")
    lines.append("<!-- Do not edit this file manually. -->")
    lines.append("")
    lines.append("# Starlark API Reference")
    lines.append("")
    lines.append("This document describes the bindings available in lore phase scripts.")
    lines.append("")

    # Overview
    lines.append("## Overview")
    lines.append("")
    lines.append("Phase scripts receive three arguments:")
    lines.append("")
    lines.append("```starlark")
    lines.append("def install(package, system, plan):")
    lines.append("    # package - context about the package being deployed")
    lines.append("    # system  - read-only queries about the current system")
    lines.append("    # plan    - actions to add to the execution graph")
    lines.append("```")
    lines.append("")
    lines.append("Additionally, a `configure(phase)` hook can set phase-level configuration.")
    lines.append("")

    # Binding summary table
    plan_methods = _count_by_kind(api["plan"], "method")
    plan_props = _count_by_kind(api["plan"], "property")
    sys_methods = _count_by_kind(api["system"], "method")
    sys_props = _count_by_kind(api["system"], "property")
    pkg_methods = _count_by_kind(api["package"], "method")
    pkg_props = _count_by_kind(api["package"], "property")
    phase_methods = _count_by_kind(api["phase"], "method")
    phase_props = _count_by_kind(api["phase"], "property")
    total_methods = plan_methods + sys_methods + pkg_methods + phase_methods
    total_props = plan_props + sys_props + pkg_props + phase_props

    lines.append("| Category | Methods | Properties | Total |")
    lines.append("|----------|---------|------------|-------|")
    lines.append("| plan.* | " + str(plan_methods) + " | " + str(plan_props) + " | " + str(plan_methods + plan_props) + " |")
    lines.append("| system.* | " + str(sys_methods) + " | " + str(sys_props) + " | " + str(sys_methods + sys_props) + " |")
    lines.append("| package.* | " + str(pkg_methods) + " | " + str(pkg_props) + " | " + str(pkg_methods + pkg_props) + " |")
    lines.append("| phase.* | " + str(phase_methods) + " | " + str(phase_props) + " | " + str(phase_methods + phase_props) + " |")
    lines.append("| **Total** | **" + str(total_methods) + "** | **" + str(total_props) + "** | **" + str(total_methods + total_props) + "** |")
    lines.append("")

    # --- plan.* ---
    lines.append("---")
    lines.append("")
    lines.append("## plan.*")
    lines.append("")
    lines.append("The `plan` object schedules actions into the execution graph.")
    lines.append("")

    # Namespace table
    lines.append("| Namespace | Purpose |")
    lines.append("|-----------|---------|")
    _NS_DESCRIPTIONS = {
        "file": "File system actions",
        "package": "Package manager actions",
        "template": "Template rendering",
        "encryption": "SOPS decryption",
        "archive": "Archive extraction",
        "git": "Git operations",
        "(root)": "Top-level plan actions",
    }
    for ns in sorted(api["plan"].keys()):
        desc = _NS_DESCRIPTIONS.get(ns, "")
        label = "plan." + ns + ".*" if ns != "(root)" else "plan.*"
        lines.append("| `" + label + "` | " + desc + " |")
    lines.append("")

    # Render each namespace
    plan_ns_order = ["file", "package", "template", "encryption", "archive", "git", "(root)"]
    for ns in plan_ns_order:
        if ns not in api["plan"]:
            continue
        entries = api["plan"][ns]
        if ns == "(root)":
            lines.append("### Top-level plan actions")
        else:
            lines.append("### plan." + ns)
        lines.append("")
        _render_entries(lines, entries)

    # --- system.* ---
    lines.append("---")
    lines.append("")
    lines.append("## system.*")
    lines.append("")
    lines.append("The `system` object provides read-only queries about the current platform state.")
    lines.append("")

    # Platform properties
    if "platform" in api["system"]:
        lines.append("### system.platform")
        lines.append("")
        platform_entries = api["system"]["platform"]
        props = [e for e in platform_entries if e.get("kind") == "property"]
        if props:
            lines.append("| Property | Type | Description |")
            lines.append("|----------|------|-------------|")
            for p in sorted(props, key=lambda e: e["full_name"]):
                lines.append("| `" + p["full_name"] + "` | " + p.get("value_type", "") + " | " + p.get("doc", "") + " |")
            lines.append("")

    # Other system namespaces
    sys_ns_order = ["package", "service", "git", "file"]
    for ns in sys_ns_order:
        if ns not in api["system"]:
            continue
        entries = api["system"][ns]
        lines.append("### system." + ns)
        lines.append("")
        _render_entries(lines, entries)

    # --- package.* ---
    lines.append("---")
    lines.append("")
    lines.append("## package.*")
    lines.append("")
    lines.append("The `package` object provides context about the package being deployed.")
    lines.append("")

    if "(root)" in api["package"]:
        entries = api["package"]["(root)"]
        props = [e for e in entries if e.get("kind") == "property"]
        methods = [e for e in entries if e.get("kind") == "method"]

        if props:
            lines.append("### Properties")
            lines.append("")
            lines.append("| Property | Type | Description |")
            lines.append("|----------|------|-------------|")
            for p in sorted(props, key=lambda e: e["full_name"]):
                lines.append("| `" + p["full_name"] + "` | " + p.get("value_type", "") + " | " + p.get("doc", "") + " |")
            lines.append("")

        if methods:
            lines.append("### Methods")
            lines.append("")
            _render_entries(lines, methods)

    # --- phase.* ---
    lines.append("---")
    lines.append("")
    lines.append("## phase.*")
    lines.append("")
    lines.append("The `phase` object is passed to `configure(phase)` hooks for phase-level configuration.")
    lines.append("")

    if "(root)" in api["phase"]:
        _render_entries(lines, api["phase"]["(root)"])

    # --- Output objects ---
    lines.append("---")
    lines.append("")
    lines.append("## Output Objects")
    lines.append("")
    lines.append("Most plan actions return an **Output** object (a promise). When passed to another")
    lines.append("action's slot, it creates an edge in the execution graph, ensuring the producer runs")
    lines.append("before the consumer.")
    lines.append("")
    lines.append("| Property | Type | Description |")
    lines.append("|----------|------|-------------|")
    lines.append("| `output.node_id` | string | Unique identifier of the producing node |")
    lines.append("| `output.slot` | string | Which output slot this represents |")
    lines.append("")
    lines.append("Use `plan.gather(*promises)` to group multiple outputs for parallel execution.")
    lines.append("When the group is passed to another action's slot, it creates edges from all")
    lines.append("members to the consumer.")
    lines.append("")

    return "\n".join(lines)


def _count_by_kind(category_dict, kind):
    """Count entries of a given kind across all namespaces in a category."""
    count = 0
    for ns in category_dict:
        for entry in category_dict[ns]:
            if entry.get("kind", "method") == kind:
                count += 1
    return count


def _render_entries(lines, entries):
    """Render a list of binding entries as markdown."""
    methods = [e for e in entries if e.get("kind", "method") == "method"]
    for entry in sorted(methods, key=lambda e: e["full_name"]):
        full = entry["full_name"]
        doc = entry.get("doc", "")
        usage = entry.get("usage", "")
        slots = entry.get("slots", [])
        slot_docs = entry.get("slot_docs", {})
        output = entry.get("output", "none")
        returns = entry.get("returns", "")

        lines.append("#### " + full)
        lines.append("")
        if doc:
            lines.append(doc)
            lines.append("")
        if usage:
            lines.append("**Usage:** `" + usage + "`")
            lines.append("")
        if slots:
            lines.append("| Slot | Description |")
            lines.append("|------|-------------|")
            for s in slots:
                s_doc = slot_docs.get(s, "")
                lines.append("| `" + s + "` | " + s_doc + " |")
            lines.append("")
        if returns:
            lines.append("**Returns:** " + returns)
            lines.append("")
        elif output == "promise":
            lines.append("**Returns:** Output (promise)")
            lines.append("")


# =============================================================================
# MIGRATION KNOWLEDGE (Writ migrate patterns for writ migrate)
# =============================================================================

def build_migration_knowledge(source, target):
    """Build migration knowledge from writ migrate source."""
    note("Building migration knowledge...")

    migrate_path = file.join(source, "internal", "writ", "migrate")
    if not file.is_directory(migrate_path):
        fail("Migrate source not found: " + migrate_path)

    execution_path = file.join(source, "internal", "execution")
    if not file.is_directory(execution_path):
        fail("Execution source not found: " + execution_path)

    knowledge_path = file.join(target, "knowledge", "migration")
    if not file.is_directory(knowledge_path):
        fail("Migration knowledge path not found: " + knowledge_path)

    # Step 1: Parse Go source files
    note("  Scanning " + migrate_path + "...")
    result = _parse_migrate_knowledge(migrate_path)

    source_systems = result["source_systems"]
    encryption_systems = result["encryption_systems"]
    repo_layers = result["repo_layers"]
    platforms = result["platforms"]

    note("  Found " + str(len(source_systems)) + " source systems")
    note("  Found " + str(len(encryption_systems)) + " encryption systems")
    note("  Found " + str(len(platforms)) + " platforms")

    # Step 1b: Scan execution package (structs, consts, ops in one pass)
    note("  Scanning " + execution_path + "...")
    execution = _scan_execution(execution_path)
    execution_ops = execution["ops"]
    note("  Found " + str(len(execution_ops)) + " execution operations")

    # Step 2: Load registry signature files
    signatures_path = file.join(knowledge_path, "signatures")
    registry_systems = []
    if file.is_directory(signatures_path):
        for entry in file.list(signatures_path):
            if entry.name.endswith(".yaml"):
                sig_path = file.join(signatures_path, entry.name)
                content = file.read(sig_path)
                sig = yaml.decode(content)
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
            aliases = p.get("aliases", [])
            for alias in aliases:
                registry_platform_aliases.append(alias)

    # Step 4: Check for contract violations
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

    # Step 7: Generate execution graph schema
    generate_execution_schema(source, knowledge_path, execution)


# Schema overrides keyed by JSON field name
_SCHEMA_OVERRIDES = {
    "operations": lambda ops: {"type": "array", "description": "Pipeline of operations to execute", "items": {"type": "string", "enum": ops}},
    "slots": lambda _: {"type": "object", "description": "Input slots for this node (name -> value)", "additionalProperties": {"$ref": "#/$defs/slot_value"}},
    "status": lambda statuses: {"type": "string", "description": "Execution status of this node", "enum": statuses},
}


def generate_execution_schema(source, knowledge_path, execution):
    """Generate engine-graph.json schema from Go struct definitions."""
    schemas_path = file.join(knowledge_path, "schemas")
    if not file.is_directory(schemas_path):
        warn("  Schemas path not found: " + schemas_path)
        return

    execution_path = file.join(source, "internal", "execution")
    note("  Generating schema from " + execution_path + "...")

    structs = execution["structs"]
    consts = execution["consts"]
    ops = execution["ops"]
    ops_list = sorted([op["name"] for op in ops])

    # Index structs and consts by name
    struct_map = {}
    for s in structs:
        struct_map[str(s.name)] = s

    const_map = {}
    for g in consts:
        values = []
        for c in g.constants:
            val = str(c.value)
            if val:
                values.append(val)
        if values:
            const_map[str(g.type_name)] = values

    # Build SlotValue schema
    slot_value_schema = {
        "type": "object",
        "description": "A slot value - either immediate or a promise (reference to another node)",
        "properties": {},
    }
    if "SlotValue" in struct_map:
        for f in struct_map["SlotValue"].fields:
            slot_value_schema["properties"][str(f.json_name)] = _field_to_json_schema(f)

    # Build Node schema
    node_properties = {}
    node_required = []
    statuses = const_map.get("NodeStatus", ["pending", "completed", "skipped", "failed"])
    if "Node" in struct_map:
        for f in struct_map["Node"].fields:
            json_name = str(f.json_name)
            if json_name in _SCHEMA_OVERRIDES:
                override_data = ops_list if json_name == "operations" else statuses
                node_properties[json_name] = _SCHEMA_OVERRIDES[json_name](override_data)
            else:
                node_properties[json_name] = _field_to_json_schema(f)
            if bool(f.required) and json_name != "status":
                node_required.append(json_name)

    # Build Edge schema
    edge_properties = {}
    edge_required = []
    if "Edge" in struct_map:
        for f in struct_map["Edge"].fields:
            json_name = str(f.json_name)
            edge_properties[json_name] = _field_to_json_schema(f)
            if bool(f.required):
                edge_required.append(json_name)

    # Build complete schema
    graph_states = const_map.get("GraphState", ["pending", "executed", "failed"])
    schema_obj = {
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
            "version": {"type": "string", "description": "Graph format version"},
            "tool": {"type": "string", "description": "Tool that created this graph"},
            "state": {"type": "string", "description": "Execution state of the graph", "enum": graph_states},
            "nodes": {"type": "array", "description": "Operations to execute", "items": {"$ref": "#/$defs/node"}},
            "edges": {"type": "array", "description": "Dependencies between nodes", "items": {"$ref": "#/$defs/edge"}},
        },
        "required": ["nodes"],
    }

    # Write schema
    engine_schema_path = file.join(schemas_path, "engine-graph.json")
    new_content = json.encode_indent(schema_obj, "  ")

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


def _field_to_json_schema(field):
    """Convert a Go struct field to JSON Schema property."""
    go_type = str(field.type)
    schema_prop = {}

    desc = str(field.description)
    if desc:
        schema_prop["description"] = desc

    if go_type == "string":
        schema_prop["type"] = "string"
    elif go_type == "int" or go_type == "int64":
        schema_prop["type"] = "integer"
    elif go_type == "bool":
        schema_prop["type"] = "boolean"
    elif go_type == "float64":
        schema_prop["type"] = "number"
    elif go_type.startswith("[]"):
        schema_prop["type"] = "array"
        inner_type = go_type[2:]
        if inner_type == "string":
            schema_prop["items"] = {"type": "string"}
        else:
            schema_prop["items"] = {"type": "object"}
    elif go_type.startswith("map["):
        schema_prop["type"] = "object"
        schema_prop["additionalProperties"] = {"type": "string"}
    elif go_type == "os.FileMode":
        schema_prop["type"] = "integer"
        schema_prop["description"] = (desc + " " if desc else "") + "(octal file permissions)"
    elif go_type == "time.Time":
        schema_prop["type"] = "string"
        schema_prop["format"] = "date-time"
    else:
        schema_prop["type"] = "string"

    return schema_prop


def check_migration_contract_violations(source_systems, encryption_systems, platforms, registry_systems, registry_platforms, registry_platform_aliases):
    """Check for contract violations between source code and registry."""
    violations = []

    source_system_values = []
    for s in source_systems:
        val = str(s["value"])
        if val not in ["unknown", "native"]:
            source_system_values.append(val)

    for system in source_system_values:
        if system not in registry_systems:
            violations.append({
                "type": "missing_signature",
                "message": "Source system '" + system + "' has no signature file in registry",
            })

    for system in registry_systems:
        if system in ["git-crypt", "sops", "age", "gpg", "blackbox", "transcrypt", "ansible-vault"]:
            continue
        found = False
        for s in source_systems:
            if str(s["value"]) == system:
                found = True
                break
        if not found:
            violations.append({
                "type": "orphan_signature",
                "message": "Registry signature '" + system + "' has no SourceSystem constant",
            })

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

    systems = []
    for s in source_systems:
        systems.append({"name": s["name"], "value": s["value"], "file": s["file"], "line": s["line"]})
    ref["source_systems"] = systems

    encryptions = []
    for e in encryption_systems:
        encryptions.append({"name": e["name"], "value": e["value"], "file": e["file"], "line": e["line"]})
    ref["encryption_systems"] = encryptions

    layers = []
    for r in repo_layers:
        layers.append({"name": r["name"], "value": r["value"], "file": r["file"], "line": r["line"]})
    ref["repo_layers"] = layers

    ref["platforms"] = [str(p) for p in platforms]
    return ref


def validate_signature_coverage(source_systems, signatures_path):
    """Validate that all source systems have proper signature files."""
    for s in source_systems:
        val = str(s["value"])
        if val in ["unknown", "native"]:
            continue

        sig_file = file.join(signatures_path, val + ".yaml")
        if not file.exists(sig_file):
            warn("  Missing signature file: " + sig_file)
        else:
            content = file.read(sig_file)
            sig = yaml.decode(content)
            if not sig.get("name"):
                warn("  Signature missing 'name': " + sig_file)
            if not sig.get("markers"):
                warn("  Signature missing 'markers': " + sig_file)


# =============================================================================
# OPS KNOWLEDGE (Operation surface mappings from *Service structs)
# =============================================================================

# Methods from starlark.Value and starlark.HasAttrs — always excluded.
_SKIP_METHODS = [
    "String", "Type", "Freeze", "Truth", "Hash",
    "Attr", "AttrNames",
]

# Common struct name suffixes to strip when deriving category
_STRIP_SUFFIXES = ["Ops", "Impl", "Service", "Handler"]


def _to_snake(name):
    """Convert CamelCase to snake_case."""
    result = []
    for i in range(len(name)):
        ch = name[i]
        if ch.isupper():
            if i > 0:
                prev = name[i - 1]
                if prev.islower():
                    result.append("_")
                elif prev.isupper() and i + 1 < len(name) and name[i + 1].islower():
                    result.append("_")
            result.append(ch.lower())
        else:
            result.append(ch)
    return "".join(result)


def build_ops_knowledge(source, target):
    """Build ops knowledge — operation surface mappings from *Service structs."""
    note("Building ops knowledge (operation surface)...")

    execution_path = file.join(source, "internal", "execution")
    if not file.is_directory(execution_path):
        fail("Execution source not found: " + execution_path)

    # Discover *Service structs
    all_structs = go.structs(execution_path)
    services = []
    for s in all_structs:
        name = str(s.name)
        if name.endswith("Service"):
            services.append(name)

    if len(services) == 0:
        note("  No *Service structs found (not yet implemented)")
        return

    note("  Found " + str(len(services)) + " service(s)")

    mappings_path = file.join(target, "knowledge", "ops", "mappings")
    file.mkdir(mappings_path)
    generated = 0

    for service_name in sorted(services):
        # Get methods for this service
        methods = go.methods(execution_path, receiver_type=service_name)

        # Filter to public methods, skip starlark.Value methods
        filtered = []
        for m in methods:
            if str(m.name)[0].islower():
                continue
            if str(m.name) in _SKIP_METHODS:
                continue
            filtered.append(m)

        if len(filtered) == 0:
            note("  " + service_name + ": no eligible methods")
            continue

        # Derive category from service name
        struct_short = service_name
        for suffix in _STRIP_SUFFIXES:
            if struct_short.endswith(suffix) and len(struct_short) > len(suffix):
                struct_short = struct_short[:-len(suffix)]
                break
        category = _to_snake(struct_short)

        # Build method descriptors
        method_descriptors = []
        for m in filtered:
            params = []
            for p in m.params:
                params.append({
                    "name": str(p.name),
                    "type": str(p.type),
                    "variadic": bool(p.variadic),
                })
            method_descriptors.append({
                "name": str(m.name),
                "returns": str(m.returns),
                "doc": str(m.doc),
                "params": params,
            })

        # Build descriptor for go.mapping()
        descriptor = {
            "package": "execution",
            "category": category,
            "struct_name": struct_short,
            "namespace": category,
            "methods": method_descriptors,
        }

        # Generate mapping YAML
        mapping_yaml = str(go.mapping(descriptor))

        # Write mapping artifact
        mapping_file = category + ".yaml"
        mapping_path = file.join(mappings_path, mapping_file)

        changes_detected = False
        if file.exists(mapping_path):
            current_content = file.read(mapping_path)
            if current_content != mapping_yaml:
                changes_detected = True
                note("  Changes detected in " + mapping_file)
        else:
            changes_detected = True
            note("  Creating new " + mapping_file)

        if changes_detected:
            file.write(mapping_path, mapping_yaml)
            success("  Wrote " + mapping_path)
        else:
            success("  No changes to " + mapping_file)

        generated += 1

    success("  Generated mappings for " + str(generated) + " service(s)")
