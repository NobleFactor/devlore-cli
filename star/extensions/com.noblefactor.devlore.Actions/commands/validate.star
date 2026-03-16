# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# validate.star - Validate devlore-cli Starlark API contract
#
# This tool validates the devlore-cli API contract and generates
# documentation for lore package developers.
#
# Contract requirements:
#   - All plan.* bindings MUST use Attr receiver methods
#   - StringDict registration is a contract violation
#
# Slot model:
#   - Any slot accepts either a promise or an immediate value
#   - Promise: creates an edge, value flows at runtime
#   - Immediate: stored directly, known at analysis time
#
# Usage:
#   star devlore ops validate --source=/path/to/devlore-cli
#   star devlore ops validate --source=/path/to/devlore-cli --format=json
#
# CI integration:
#   Exits with non-zero status if contract violations are found.
#   Use --format=json for machine-readable output.

def run(ctx):
    """Main entry point."""
    source = ctx.args.get("source", "")
    output_format = ctx.args.get("format", "markdown")

    if source == "":
        fail("--source is required: path to devlore-cli repository")
        return

    # Path to the starlark package
    starlark_path = file.join(source, "internal", "starlark")

    if not file.exists(starlark_path):
        fail("Starlark package not found at " + starlark_path)
        return

    # Parse the API using Go AST primitives
    api = _parse_devlore_api(starlark_path)

    # Count bindings for display
    binding_count = _count_bindings(api)
    violation_count = len(api["violations"])

    if output_format == "json":
        print(json.encode(api))
    elif output_format == "yaml":
        print(yaml.encode(api))
    else:
        _output_markdown(api, binding_count, violation_count)

    if violation_count > 0:
        fail("Contract violations found: " + str(violation_count))


# =============================================================================
# GO AST HELPERS
# =============================================================================

def _parse_devlore_api(path):
    """Parse devlore-cli Starlark API from Go source files.

    Uses goast.methods() and goast.calls() to extract:
    - Binding names from Attr methods (via NewBuiltin calls)
    - Handler details: doc, slots, operations, output
    - StringDict violations (non-Attr NewBuiltin registrations)
    """
    attr_methods = goast.methods(path, name="Attr")
    bindings = {}
    seen = {}

    for attr_method in attr_methods:
        # Look for both direct NewBuiltin and MakeAttr (which wraps NewBuiltin)
        for call_name in ["NewBuiltin", "MakeAttr"]:
            builtin_calls = goast.calls(attr_method.scope, name=call_name)
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
                    bindings[binding_name] = binding

    # Detect StringDict violations
    all_methods = goast.methods(path)
    violations = []
    for method in all_methods:
        if str(method.name) == "Attr":
            continue
        nb_calls = goast.calls(method.scope, name="NewBuiltin")
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

    # Build hierarchical result
    plan = {}
    system = {}
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
        if context == "plan":
            if namespace not in plan:
                plan[namespace] = []
            plan[namespace].append(entry)
        elif context == "system":
            if namespace not in system:
                system[namespace] = []
            system[namespace].append(entry)

    return {
        "valid": len(violations) == 0,
        "plan": plan,
        "system": system,
        "violations": violations,
    }


def _is_api_binding(name):
    """Check if a binding name is an API binding (plan.* or system.*)."""
    return name.startswith("plan.") or name.startswith("system.")


def _extract_binding_info(path, handler_name, binding_name, fallback_file, fallback_line):
    """Extract binding info from a handler method."""
    if not handler_name:
        return {"file": fallback_file, "line": fallback_line}

    handler_methods = goast.methods(path, name=handler_name)
    if len(list(handler_methods)) == 0:
        return {"file": fallback_file, "line": fallback_line}

    handler = list(handler_methods)[0]
    scope = str(handler.scope)

    doc, usage, slot_docs, returns = _parse_doc_comment(str(handler.doc))

    # Extract slots from FillSlot calls
    slots = []
    slot_seen = {}
    fill_calls = goast.calls(scope, name="FillSlot")
    for call in fill_calls:
        args = list(call.args)
        if len(args) >= 3:
            slot_name = str(args[2].string_value)
            if slot_name and slot_name not in slot_seen:
                slot_seen[slot_name] = True
                slots.append(slot_name)

    # Extract operations from execution.Node composites
    operations = []
    node_composites = goast.composites(scope, type_name="execution.Node")
    for comp in node_composites:
        ops_field = getattr(comp.fields, "Operations", None)
        if ops_field:
            for op in ops_field:
                op_str = str(op)
                if op_str not in operations:
                    operations.append(op_str)

    # Detect output type
    output = "none"
    output_calls = goast.calls(scope, name="NewOutput")
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


# =============================================================================
# OUTPUT HELPERS
# =============================================================================

def _count_bindings(api):
    """Count total bindings in the API dict."""
    count = 0
    for ns in api["plan"]:
        count += len(api["plan"][ns])
    for ns in api["system"]:
        count += len(api["system"][ns])
    return count


def _output_markdown(api, binding_count, violation_count):
    """Output API documentation in Markdown format."""
    print("# devlore-cli Starlark API")
    print("")

    if violation_count > 0:
        print("## Contract Violations")
        print("")
        print("> **ERROR**: The following bindings use StringDict instead of Attr receivers:")
        print("")
        for v in api["violations"]:
            print("- `" + v["name"] + "` (" + v["file"] + ":" + str(v["line"]) + ")")
        print("")

    print("## Slot Model")
    print("")
    print("Any slot accepts either a **promise** or an **immediate** value:")
    print("")
    print("| Type | Behavior |")
    print("|------|----------|")
    print("| Promise | Creates an edge; value flows at runtime |")
    print("| Immediate | Stored directly; known at analysis time |")
    print("")

    print("## plan")
    print("")
    print("*Execution graph builders - all mutations go through plan.*")
    print("")
    _output_context_markdown(api["plan"])

    print("## system")
    print("")
    print("*Read-only system state queries.*")
    print("")
    _output_context_markdown(api["system"])

    print("---")
    print("")
    print("*Valid bindings: " + str(binding_count) + "*")


def _output_context_markdown(context):
    """Output namespaces and methods for a context (dict)."""
    namespaces = sorted(context.keys())

    for namespace in namespaces:
        methods = context[namespace]
        if namespace == "(root)":
            print("### (root methods)")
        else:
            print("### " + namespace)
        print("")

        for m in methods:
            print("#### `" + m["name"] + "()`")
            print("")

            if m["doc"]:
                print(m["doc"])
                print("")

            if m["usage"]:
                print("```python")
                print(m["usage"])
                print("```")
                print("")

            slots = m["slots"]
            if len(slots) > 0:
                print("| Slot | Description |")
                print("|------|-------------|")
                for slot in slots:
                    slot_doc = m["slot_docs"].get(slot, "")
                    print("| `" + slot + "` | " + (slot_doc if slot_doc else "-") + " |")
                print("")

            ops = m["operations"]
            ops_str = ", ".join(["`" + op + "`" for op in ops]) if len(ops) > 0 else "none"
            print("**Operations**: " + ops_str + "  ")
            print("**Output**: " + m["output"])
            if m["returns"]:
                print("  ")
                print("**Returns**: " + m["returns"])
            print("")
