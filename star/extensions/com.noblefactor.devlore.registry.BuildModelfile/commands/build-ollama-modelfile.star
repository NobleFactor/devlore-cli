# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

#
# build-ollama-modelfile.star - Generate Ollama Modelfiles from knowledge domains
#
# This operation assembles knowledge assets (prompts, signatures, schemas,
# examples) into an Ollama Modelfile with a comprehensive SYSTEM prompt.
#
# Usage:
#   star devlore build-ollama-modelfile --domain migration --model qwen3:8b
#   star devlore build-ollama-modelfile --domain migration --create --name devlore-migrate
#   star devlore build-ollama-modelfile --list

# Default base model for Modelfiles
DEFAULT_MODEL = "qwen3:8b"

# Asset types to include in SYSTEM prompt (in order)
SYSTEM_ASSET_ORDER = [
    ("prompts", "MAIN PROMPT", ".txt"),
    ("concepts", "CONCEPTS", ".yaml"),
    ("signatures", "DETECTION SIGNATURES", ".yaml"),
    ("transforms", "TRANSFORM RULES", ".yaml"),
    ("examples", "FEW-SHOT EXAMPLES", ".yaml"),
    ("schemas", "OUTPUT SCHEMA", ".json"),
]

# =============================================================================
# MODELFILE GENERATION
# =============================================================================

def build_modelfile(knowledge_path, domain, model):
    """Assemble Modelfile from knowledge assets."""
    lines = []

    # Header
    lines.append("# Generated Modelfile for domain: " + domain)
    lines.append("# Generated: " + _timestamp())
    lines.append("# Source: " + knowledge_path)
    lines.append("")
    lines.append("FROM " + model)
    lines.append("")
    lines.append('SYSTEM """')

    # Include each asset type in order
    for asset_type, section_name, extension in SYSTEM_ASSET_ORDER:
        asset_dir = file.join(knowledge_path, asset_type)
        if not file.is_directory(asset_dir):
            continue

        section_content = _build_section(asset_dir, section_name, extension)
        if section_content:
            lines.append(section_content)

    # Close SYSTEM prompt
    lines.append('"""')
    lines.append("")

    # Parameters optimized for structured output
    lines.append("PARAMETER temperature 0.1")
    lines.append("PARAMETER num_ctx 32768")
    lines.append("")

    return "\n".join(lines)


def _build_section(asset_dir, section_name, extension):
    """Build a section of the SYSTEM prompt from asset files."""
    files = _list_asset_files(asset_dir, extension)
    if not files:
        return ""

    lines = []
    lines.append("# === " + section_name + " ===")
    lines.append("")

    for filename in files:
        filepath = file.join(asset_dir, filename)
        content = file.read(filepath)

        # Skip baseline/generated files in examples
        if filename.startswith("baseline-"):
            continue

        # Determine how to format based on extension
        name = filename
        if extension:
            name = filename.replace(extension, "")

        if extension == ".txt":
            # Plain text - include directly
            lines.append(content)
        elif extension == ".yaml" or extension == ".json":
            # Structured data - wrap in code fence
            lines.append("## " + name)
            lines.append("")
            lines.append("```" + extension.replace(".", ""))
            lines.append(content.strip())
            lines.append("```")
            lines.append("")

    return "\n".join(lines)


def _list_asset_files(dir_path, extension):
    """List files in directory matching extension."""
    files = []
    if not file.exists(dir_path):
        return files

    for entry in file.list(dir_path):
        if entry.is_dir:
            continue
        if entry.name.startswith("."):
            continue
        if extension and not entry.name.endswith(extension):
            continue
        files.append(entry.name)

    return sorted(files)


def _timestamp():
    """Get current UTC timestamp placeholder."""
    # Starlark runtime doesn't have time module or shell.run
    # Timestamp will be filled in at runtime if needed
    return "(generated)"


def _find_registry(explicit_path):
    """Find devlore-registry path."""
    if explicit_path:
        return explicit_path

    # Try sibling directory
    sibling = file.join("..", "devlore-registry")
    if file.is_directory(sibling):
        return sibling

    # Try current directory
    if file.is_directory("knowledge"):
        return "."

    return ""


def _list_knowledge_domains(registry):
    """List all knowledge domains in the registry."""
    knowledge_path = file.join(registry, "knowledge")
    if not file.is_directory(knowledge_path):
        return []

    domains = []
    for entry in file.list(knowledge_path):
        if entry.is_dir and not entry.name.startswith("."):
            domains.append(entry.name)
    return sorted(domains)


def _build_domain(registry, domain, model):
    """Build Modelfile for a single domain."""
    knowledge_path = file.join(registry, "knowledge", domain)
    if not file.is_directory(knowledge_path):
        fail("Knowledge domain not found: " + domain)

    content = build_modelfile(knowledge_path, domain, model)
    output_path = "Modelfile." + domain

    file.write(output_path, content)
    success("Generated: " + output_path)

    # Show stats
    lines = content.count("\n")
    note("  Model: " + model)
    note("  Lines: " + str(lines))


# =============================================================================
# COMMANDS
# =============================================================================

def run(ctx):
    """Generate Modelfile from knowledge domain."""
    registry_path = ctx.args.get("registry_path", "")
    domain = ctx.args.get("domain", "all")
    model = ctx.args.get("model", DEFAULT_MODEL)

    registry = _find_registry(registry_path)
    if not registry:
        fail("Cannot find devlore-registry. Use --registry_path to specify location.")

    # Determine which domains to build
    if domain == "all":
        domains = ["migration", "onboarding"]
    else:
        domains = [domain]

    for d in domains:
        _build_domain(registry, d, model)