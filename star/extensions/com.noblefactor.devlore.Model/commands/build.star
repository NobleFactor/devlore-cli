# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

#
# build.star - Generate Ollama Modelfiles from knowledge domains
#
# This operation assembles knowledge assets (prompts, signatures, schemas,
# examples) into an Ollama Modelfile with a comprehensive SYSTEM prompt.
#
# Usage:
#   star devlore model build --domain migration --model qwen3:8b
#   star devlore model build --target=/tmp/out

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
        if not file.is_dir(asset_dir):
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
        content = file.read_text(filepath)

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


def _list_asset_files(dir_path, extension=""):
    """List files in directory matching extension (non-recursive)."""
    if not file.exists(dir_path):
        return []

    files = []
    pattern = "*" + extension if extension else "*"
    for path in file.glob(file.join(dir_path, pattern)):
        name = file.name(path)
        if not file.is_dir(path) and not name.startswith("."):
            files.append(name)
    return sorted(files)


def _timestamp():
    """Get current UTC timestamp placeholder."""
    return "(generated)"


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


def _list_knowledge_domains(target):
    """List all knowledge domains in the target."""
    knowledge_path = file.join(target, "knowledge")
    if not file.is_dir(knowledge_path):
        return []

    domains = []
    for path in file.glob(file.join(knowledge_path, "*")):
        name = file.name(path)
        if file.is_dir(path) and not name.startswith("."):
            domains.append(name)
    return sorted(domains)


def _build_domain(target, domain, model):
    """Build Modelfile for a single domain."""
    knowledge_path = file.join(target, "knowledge", domain)
    if not file.is_dir(knowledge_path):
        fail("Knowledge domain not found: " + domain)

    content = build_modelfile(knowledge_path, domain, model)
    output_path = file.join(target, "Modelfile." + domain)

    file.write_text(output_path, content)
    ui.success("Generated: " + output_path)

    # Show stats
    lines = content.count("\n")
    ui.note("  Model: " + model)
    ui.note("  Lines: " + str(lines))


# =============================================================================
# COMMANDS
# =============================================================================

def run(command, ctx):
    """Generate Modelfile from knowledge domain."""
    target = _resolve_target(ctx)
    domain = ctx.args.get("domain", "all")
    model = ctx.args.get("model", DEFAULT_MODEL)

    # Determine which domains to build
    if domain == "all":
        domains = ["migration", "onboarding"]
    else:
        domains = [domain]

    for d in domains:
        _build_domain(target, d, model)
