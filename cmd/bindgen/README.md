# bindgen - Starlark Binding Generator

Generates Starlark bindings from CLI metadata.

## Status

**Proof of concept.** This is an experiment to explore automated binding generation.

## Usage

```bash
# Build
go build -o bindgen ./cmd/bindgen

# Parse CLI --help output to YAML definition
./bindgen parse docker run build push > docker.yaml

# Edit docker.yaml to refine:
# - Fix inferred types (--help doesn't always indicate types clearly)
# - Add descriptions
# - Remove flags you don't want to expose
# - Add return type parsing hints

# Generate Go code and Starlark stubs from definition
./bindgen generate docker.yaml
# -> docker_gen.go (Starlark bindings)
# -> docker_gen.star (IDE completion stubs)

# Or quick scaffold without manual refinement
./bindgen scaffold gh repo issue pr
```

## Workflow

1. **Parse**: Extract flag names and inferred types from `--help`
2. **Refine**: Edit YAML to correct types, add docs, curate surface area
3. **Generate**: Produce Go bindings and Starlark stubs

The YAML step is intentional. Auto-parsing `--help` gets you 60-70% of the way, but:
- Type inference from help text is imprecise
- You probably don't want all 50 flags exposed
- Return type parsing (json, lines, none) requires human knowledge
- Good documentation requires human writing

## Definition Format

```yaml
name: docker
description: Docker container runtime

commands:
  run:
    description: Run a container
    args:
      - name: image
        type: string
        required: true
        position: 0
    flags:
      - name: detach
        short: d
        type: bool
        description: Run in background
      - name: publish
        short: p
        type: string_list
        description: Port mappings (host:container)
    returns:
      type: result
      fields: [ok, stdout, stderr, code]
```

### Types

| Type | Go | Starlark | Notes |
|------|-----|----------|-------|
| `string` | `string` | `str` | Most flags |
| `int` | `int` | `int` | Numeric values |
| `bool` | `bool` | `bool` | Flags without values |
| `string_list` | `[]string` | `list` | Repeatable flags (-p 80 -p 443) |
| `string_map` | `map[string]string` | `dict` | Key=value flags |

### Return Types

- `result`: Standard {ok, stdout, stderr, code} struct
- `string`: Stdout only
- `bool`: Success/failure
- `list`: Parse stdout as lines
- `dict`: Parse stdout as JSON

## Prior Art

This is not a novel idea. See:

- **Stargo** (google/starlark-go): Exposes Go packages to Starlark via reflection
- **starlark crate (Rust)**: Uses `#[starlark_module]` proc macro for bindings
- **cli-wrapper (Python)**: Wraps CLI tools as Python classes
- **OpenAPI generators**: Generate CLIs and SDKs from API specs

Key difference: We're wrapping CLI commands, not Go packages or REST APIs.
The input is `--help` output, not reflection or OpenAPI specs.

## Limitations

- `--help` parsing is heuristic and will miss things
- No support for subcommand hierarchies (docker compose up)
- Generated code needs manual review before production use
- No automated testing of generated bindings

## Future Directions

- Parse fish/zsh completions for better flag enumeration
- Support OpenAPI specs where CLI wraps an API (e.g., gh, kubectl)
- Man page parsing for richer documentation
- Integration with existing binding definitions (merge parsed + manual)
