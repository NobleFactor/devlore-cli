# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# api.star - Static analysis tool for devlore-cli Starlark API
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
#   star devlore-cli api --path=/path/to/devlore-cli
#   star devlore-cli api --path=/path/to/devlore-cli --format=json
#
# CI integration:
#   Exits with non-zero status if contract violations are found.
#   Use --format=json for machine-readable output.

def run(ctx):
    """Main entry point."""
    devlore_path = ctx.args.get("path", "")
    output_format = ctx.args.get("format", "markdown")

    if devlore_path == "":
        fail("--path is required: path to devlore-cli repository")
        return

    # Path to the starlark package
    starlark_path = file.join(devlore_path, "internal", "starlark")

    if not file.exists(starlark_path):
        fail("Starlark package not found at " + starlark_path)
        return

    # Parse the API using Go AST - returns hierarchical structure directly
    api = go.parse_devlore_api(starlark_path)

    # Count bindings for display
    binding_count = _count_bindings(api)
    violation_count = len(list(api.violations))

    if output_format == "json":
        print(json.encode(_api_to_dict(api)))
    elif output_format == "yaml":
        print(yaml.encode(_api_to_dict(api)))
    else:
        _output_markdown(api, binding_count, violation_count)

    if violation_count > 0:
        fail("Contract violations found: " + str(violation_count))


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
    """Convert API struct to dict for JSON/YAML serialization."""
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


def _output_markdown(api, binding_count, violation_count):
    """Output API documentation in Markdown format.

    Uses heading levels to reflect hierarchy:
        # API title
        ## context (plan, system)
        ### namespace (file, package, etc.)
        #### method
    """
    print("# devlore-cli Starlark API")
    print("")

    # Show violations first
    if violation_count > 0:
        print("## Contract Violations")
        print("")
        print("> **ERROR**: The following bindings use StringDict instead of Attr receivers:")
        print("")
        for v in api.violations:
            print("- `" + v.name + "` (" + v.file + ":" + str(v.line) + ")")
        print("")

    # Slot model explanation
    print("## Slot Model")
    print("")
    print("Any slot accepts either a **promise** or an **immediate** value:")
    print("")
    print("| Type | Behavior |")
    print("|------|----------|")
    print("| Promise | Creates an edge; value flows at runtime |")
    print("| Immediate | Stored directly; known at analysis time |")
    print("")

    # plan context (mutations)
    print("## plan")
    print("")
    print("*Execution graph builders - all mutations go through plan.*")
    print("")
    _output_context_markdown(api.plan)

    # system context (queries)
    print("## system")
    print("")
    print("*Read-only system state queries.*")
    print("")
    _output_context_markdown(api.system)

    print("---")
    print("")
    print("*Valid bindings: " + str(binding_count) + "*")


def _output_context_markdown(context):
    """Output namespaces and methods for a context (struct)."""
    namespaces = sorted([ns for ns in dir(context) if not ns.startswith("_")])

    for namespace in namespaces:
        methods = list(getattr(context, namespace))
        if namespace == "(root)":
            print("### (root methods)")
        else:
            print("### " + namespace)
        print("")

        for m in methods:
            print("#### `" + m.name + "()`")
            print("")

            if m.doc:
                print(m.doc)
                print("")

            if m.usage:
                print("```python")
                print(m.usage)
                print("```")
                print("")

            # Slots table
            slots = list(m.slots)
            if len(slots) > 0:
                print("| Slot | Description |")
                print("|------|-------------|")
                for slot in slots:
                    slot_doc = getattr(m.slot_docs, slot, "")
                    print("| `" + slot + "` | " + (slot_doc if slot_doc else "-") + " |")
                print("")

            # Operations and output
            ops = list(m.operations)
            ops_str = ", ".join(["`" + op + "`" for op in ops]) if len(ops) > 0 else "none"
            print("**Operations**: " + ops_str + "  ")
            print("**Output**: " + m.output)
            if m.returns:
                print("  ")
                print("**Returns**: " + m.returns)
            print("")