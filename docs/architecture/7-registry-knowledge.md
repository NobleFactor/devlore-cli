# Registry Knowledge Base Architecture

This document describes the knowledge base structure in the devlore-registry
and how CLI commands consume it for LLM-assisted operations.

## Overview

The devlore-registry contains two types of content:

1. **Package Manifests** (`packages/`) — Installation lifecycles for software
2. **Knowledge Base** (`knowledge/`) — Prompts, schemas, and patterns for LLM operations

This document focuses on the knowledge base.

## Directory Structure

```
devlore-registry/
├── packages/           # Package manifests (not covered here)
└── knowledge/
    ├── migration/      # writ migrate knowledge
    │   ├── index.yaml
    │   ├── prompts/
    │   ├── signatures/
    │   ├── examples/
    │   └── transforms/
    └── onboarding/     # lore onboard knowledge
        ├── index.yaml
        ├── prompts/
        ├── schemas/
        └── examples/
```

## Knowledge Domains

Each domain has an `index.yaml` that declares its assets:

```yaml
# knowledge/migration/index.yaml
domain: migration
prompts:
  - name: migrate-to-writ.txt
schemas:
  - name: migration-output.json
signatures:
  - name: dotfile-systems.yaml
  - name: tuckr.yaml
  - name: stow.yaml
  - name: chezmoi.yaml
examples:
  - name: tuckr-to-writ.yaml
transforms:
  - name: platform-normalization.yaml
```

### Asset Types

| Type | Purpose | Format |
|------|---------|--------|
| `prompts` | LLM instruction templates | Text with Go template syntax |
| `schemas` | Output structure definitions | JSON Schema or YAML |
| `signatures` | Detection patterns for systems | YAML with confidence scores |
| `examples` | Sample inputs/outputs for LLM context | YAML |
| `transforms` | Transformation rules | YAML |
| `slots` | Reusable prompt fragments | Text |

## Migration Domain

### Prompts

**`migrate-to-writ.txt`** — Main prompt for dotfiles migration analysis.

```
You are analyzing a dotfiles repository for migration to writ conventions.

## Writ Conventions
- Projects live in Home/Configs/ or at repository root
- Platform variants use dot separator: <project>.<Platform>
- Known platforms: Darwin, Linux, Unix, Windows, Debian, Ubuntu, RHEL, Fedora, Arch

## System Detection Signatures
{{.Signatures}}

## Your Task
Analyze the repository and produce:
1. System identification (tuckr, stow, chezmoi, etc.)
2. Structure analysis (where groups live, naming conventions)
3. Execution graph with rename operations

{{.InputSection}}
```

### Signatures

Detection patterns for dotfile management systems.

**`dotfile-systems.yaml`** — Comprehensive detection signatures:

```yaml
systems:
  tuckr:
    description: "Tuckr organizes dotfiles in Configs/ with platform suffixes"
    markers:
      - type: directory
        path: "Configs/"
        confidence: 0.9
      - type: directory
        path: "Hooks/"
        confidence: 0.95
      - type: directory_pattern
        pattern: "*_{linux,macos,darwin,windows}"
        confidence: 0.95
      - type: script_content
        pattern: "tuckr add"
        confidence: 1.0
    platform_transform:
      from: "{name}_{platform}"
      to: "{name}.{Platform}"
      mapping:
        linux: Linux
        macos: Darwin
        darwin: Darwin
        windows: Windows
        unix: Unix

  stow:
    description: "GNU Stow uses package directories with home-relative paths"
    markers:
      - type: file
        path: ".stow-local-gitignore"
        confidence: 1.0
      - type: script_content
        pattern: "stow -t ~"
        confidence: 0.95
    platform_support: none

  chezmoi:
    description: "chezmoi encodes metadata in filenames"
    markers:
      - type: file_pattern
        pattern: "dot_*"
        confidence: 0.95
      - type: file_pattern
        pattern: "run_once_*"
        confidence: 1.0
      - type: file_content
        pattern: "{{ .chezmoi."
        confidence: 1.0
```

### Examples

**`tuckr-to-writ.yaml`** — Example transformation:

```yaml
input:
  structure:
    - Home/Configs/all/
    - Home/Configs/all_darwin/
    - Home/Configs/all_linux/
    - Home/Configs/noblefactor/
    - Home/Configs/noblefactor_unix/

output:
  renames:
    - from: Home/Configs/all_darwin
      to: Home/Configs/all.Darwin
    - from: Home/Configs/all_linux
      to: Home/Configs/all.Linux
    - from: Home/Configs/noblefactor_unix
      to: Home/Configs/noblefactor.Unix
```

## Onboarding Domain

### Prompts

**`discover-product.txt`** — Extract installation requirements from documentation:

```
You are analyzing a software product page to extract installation requirements.

## Task
From the provided content, extract:
1. Product name and description
2. Installation methods per platform
3. Required dependencies
4. Configuration files that should be managed

## Output Schema
{{.Schema}}

## Content to Analyze
{{.Content}}
```

**`init-environment.txt`** — Generate environment initialization plan:

```
You are creating an environment initialization plan for a new developer workstation.

## Available Packages
{{.AvailablePackages}}

## Target Platform
{{.Platform}}

## User Requirements
{{.Requirements}}

## Task
Generate a deployment plan that:
1. Resolves dependencies in correct order
2. Identifies platform-specific packages
3. Groups related configurations
```

### Schemas

**`init-plan.json`** — JSON Schema for initialization output:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["packages", "configs"],
  "properties": {
    "packages": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "source": {"enum": ["registry", "brew", "apt", "winget"]},
          "features": {"type": "array", "items": {"type": "string"}}
        }
      }
    },
    "configs": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "path": {"type": "string"},
          "template": {"type": "boolean"},
          "content": {"type": "string"}
        }
      }
    }
  }
}
```

**`installation-patterns.yaml`** — Common installation patterns:

```yaml
categories:
  cli_tool:
    description: "Command-line utilities"
    common_sources: [brew, apt, cargo, go]
    examples: [ripgrep, fd, jq, yq]

  runtime:
    description: "Language runtimes and version managers"
    common_sources: [brew, apt, asdf, mise]
    examples: [node, python, go, rust]
    complexity: medium

  gui_app:
    description: "Desktop applications"
    common_sources: [brew-cask, apt, winget, flatpak]
    examples: [vscode, firefox, slack]
    complexity: low

  system_service:
    description: "Background services and daemons"
    common_sources: [brew, apt, docker]
    examples: [postgresql, redis, docker]
    complexity: high
```

## Loading Knowledge

### Go API

```go
package lorepackage

// LoadPrompt loads a prompt template from the knowledge base.
func (r *Registry) LoadPrompt(domain, name string) (string, error) {
    path := filepath.Join(r.Path, "knowledge", domain, "prompts", name)
    content, err := os.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("load prompt %s/%s: %w", domain, name, err)
    }
    return string(content), nil
}

// LoadSignatures loads detection signatures for a domain.
func (r *Registry) LoadSignatures(domain string) ([]Signature, error) {
    indexPath := filepath.Join(r.Path, "knowledge", domain, "index.yaml")
    // Parse index.yaml, load each signature file
    // ...
}

// ExecutePrompt loads and executes a prompt template with data.
func (r *Registry) ExecutePrompt(domain, name string, data any) (string, error) {
    content, err := r.LoadPrompt(domain, name)
    if err != nil {
        return "", err
    }

    tmpl, err := template.New(name).Parse(content)
    if err != nil {
        return "", fmt.Errorf("parse prompt template: %w", err)
    }

    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", fmt.Errorf("execute prompt template: %w", err)
    }
    return buf.String(), nil
}
```

### Usage Example

```go
func BuildMigrationPrompt(reg *lorepackage.Registry, input *GatherInput) (string, error) {
    // Load signatures to include in prompt
    sigs, err := reg.LoadSignatures("migration")
    if err != nil {
        return "", err
    }

    // Execute prompt template with data
    return reg.ExecutePrompt("migration", "migrate-to-writ.txt", map[string]any{
        "Signatures":   formatSignatures(sigs),
        "InputSection": formatInput(input),
    })
}
```

## Index Generation

The `index.yaml` files can be auto-generated by noblefactor-ops:

```bash
nf-ops index-knowledge
```

This scans each domain directory and creates/updates the index with all discovered assets.

### Index Schema

```yaml
domain: string           # Domain name (migration, onboarding)
prompts:                  # Prompt templates
  - name: string          # Filename
    description: string   # Optional description
schemas:                  # Output schemas
  - name: string
signatures:               # Detection patterns
  - name: string
examples:                 # Example inputs/outputs
  - name: string
transforms:               # Transformation rules
  - name: string
slots:                    # Reusable prompt fragments
  - name: string
```

## Versioning

Knowledge assets follow the registry versioning scheme:

1. **Prompts** are versioned implicitly with the registry
2. **Schemas** should use JSON Schema `$id` for explicit versioning
3. **Breaking changes** to output schemas require CLI version bumps

### Compatibility

The CLI embeds a minimum-compatible registry version:

```go
const MinRegistryVersion = "0.5.0"

func (r *Registry) CheckCompatibility() error {
    version, err := r.Version()
    if err != nil {
        return err
    }
    if semver.Compare(version, MinRegistryVersion) < 0 {
        return fmt.Errorf("registry version %s < minimum %s", version, MinRegistryVersion)
    }
    return nil
}
```

## Extending the Knowledge Base

### Adding a New Domain

1. Create directory: `knowledge/<domain>/`
2. Create `index.yaml` with domain name
3. Add prompt templates in `prompts/`
4. Add schemas in `schemas/`
5. Run `nf-ops index-knowledge` to update index

### Adding Detection Signatures

1. Create signature file: `knowledge/migration/signatures/<system>.yaml`
2. Follow the signature schema (markers, confidence, platform_transform)
3. Update `index.yaml` or run `nf-ops index-knowledge`
4. Test with E2E fixtures

### Adding Examples

Examples serve as few-shot learning context for the LLM:

1. Create example file: `knowledge/<domain>/examples/<name>.yaml`
2. Include both `input` and `output` sections
3. Update index
4. Reference in prompts: `{{range .Examples}}...{{end}}`

---

## Appendix — Package Signatures (design sketch, relocated 2026-08-20)

> Relocated verbatim from `docs/sketches/package-signatures.md` during the resource-construction sketch
> retirement ([#582](https://github.com/NobleFactor/devlore-cli/issues/582)); `Epic:LorePackaging` owns its
> maturation.

## Overview

Package signatures enable the registry to recognize installation attempts in scripts,
URLs, and commands. This inverts the current model: instead of the migrate tool
knowing about package managers, the registry knows how each package gets installed.

## Manifest Schema

```yaml
name: ripgrep
version: "14.1.0"
description: Fast regex search tool

# Aliases this package is known by
aliases:
  - rg

# Installation signatures by method
signatures:
  # Package manager → package names
  brew: [ripgrep, rg]
  apt: [ripgrep]
  cargo: [ripgrep]
  scoop: [ripgrep]
  choco: [ripgrep]
  nix: [ripgrep]

  # URL patterns (regex) for curl|bash or download detection
  urls:
    - 'github\.com/BurntSushi/ripgrep'
    - 'ripgrep.*\.tar\.gz'

  # Command patterns (regex) for unusual install methods
  commands:
    - 'cargo install ripgrep'
    - 'cargo binstall ripgrep'

# Standard lore manifest follows...
platforms:
  darwin:
    install: brew install ripgrep
  linux:
    install: |
      apt update && apt install -y ripgrep
```

## Registry API

### Signature Search

```
GET /v1/signatures/search?q=ripgrep
GET /v1/signatures/search?manager=brew&package=rg
GET /v1/signatures/search?url=github.com/BurntSushi/ripgrep
```

Response:
```json
{
  "matches": [
    {
      "package": "ripgrep",
      "confidence": 1.0,
      "match_type": "exact",
      "match_field": "signatures.brew"
    }
  ]
}
```

### Bulk Signature Lookup

For efficiency, migrate tool sends batch of candidates:

```
POST /v1/signatures/lookup
Content-Type: application/json

{
  "candidates": [
    {"manager": "brew", "names": ["ripgrep", "fd", "bat", "unknown-pkg"]},
    {"manager": "apt", "names": ["neovim", "tmux"]},
    {"url": "https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh"}
  ]
}
```

Response:
```json
{
  "resolved": {
    "ripgrep": {"package": "ripgrep", "confidence": 1.0},
    "fd": {"package": "fd", "confidence": 1.0},
    "bat": {"package": "bat", "confidence": 1.0},
    "neovim": {"package": "neovim", "confidence": 1.0},
    "tmux": {"package": "tmux", "confidence": 1.0},
    "https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh": {
      "package": "nvm",
      "confidence": 0.95,
      "match_type": "url_pattern"
    }
  },
  "unresolved": ["unknown-pkg"]
}
```

## Migrate Tool Changes

### Current Flow

```
script line → regex match → (manager, package_name) → observation string
```

### New Flow

```
script line → generic extraction → candidates[] → registry lookup → resolved packages → migration plan
```

### Generic Extraction

Replace manager-specific regexes with generic patterns:

```go
type extractedCandidate struct {
    Manager string   // "brew", "apt", "cargo", "", etc.
    Names   []string // package names found
    URL     string   // if curl/wget detected
    Line    int
    Raw     string   // original line
}

var extractors = []struct {
    pattern   *regexp.Regexp
    manager   string
    nameGroup int
    multi     bool
}{
    {regexp.MustCompile(`brew\s+install\s+(.+)`), "brew", 1, true},
    {regexp.MustCompile(`apt(?:-get)?\s+install\s+(.+)`), "apt", 1, true},
    {regexp.MustCompile(`cargo\s+install\s+(\S+)`), "cargo", 1, false},
    {regexp.MustCompile(`pip3?\s+install\s+(\S+)`), "pip", 1, false},
    {regexp.MustCompile(`npm\s+install\s+-g\s+(\S+)`), "npm", 1, false},
    {regexp.MustCompile(`go\s+install\s+(\S+)`), "go", 1, false},
    {regexp.MustCompile(`curl\s+.*?(https?://\S+)`), "", 0, false}, // URL extraction
    {regexp.MustCompile(`wget\s+.*?(https?://\S+)`), "", 0, false},
}
```

### Registry Client

```go
type SignatureClient struct {
    baseURL string
    cache   *signatureCache
}

func (c *SignatureClient) Lookup(candidates []extractedCandidate) (*LookupResult, error)

type LookupResult struct {
    Resolved   map[string]ResolvedPackage
    Unresolved []string
}

type ResolvedPackage struct {
    Name       string
    Confidence float64
    MatchType  string // "exact", "alias", "url_pattern", "command_pattern"
}
```

### Enhanced Script Analysis

```go
type ScriptAnalysis struct {
    RelPath    string
    Name       string
    Phase      string
    LineCount  int

    // New: resolved packages with lore equivalents
    Packages []AnalyzedPackage

    // New: unresolved installations (no lore package found)
    Unknown []UnknownInstall
}

type AnalyzedPackage struct {
    LorePackage string   // registry package name
    Confidence  float64
    SourceLine  int
    SourceCmd   string   // original command
    Manager     string   // how it was being installed
}

type UnknownInstall struct {
    Line    int
    Command string
    Manager string
    Names   []string // attempted package names
}
```

## Migration Output Enhancement

Before:
```
Installs ripgrep, fd, bat via brew
```

After:
```
Found 3 packages with lore equivalents:
  Line 12: brew install ripgrep  →  lore deploy ripgrep
  Line 13: brew install fd       →  lore deploy fd
  Line 14: brew install bat      →  lore deploy bat

Found 1 unknown package (no lore manifest):
  Line 15: brew install some-obscure-tool

Suggested migration:
  lore deploy ripgrep fd bat
```

## Offline Support

The migrate tool should work offline with degraded functionality:

1. **Bundled signatures**: Ship common package signatures with CLI
2. **Local cache**: Cache registry lookups in `~/.local/state/devlore/signatures/`
3. **Graceful degradation**: If registry unavailable, fall back to bundled + cached

```go
func (c *SignatureClient) Lookup(candidates []extractedCandidate) (*LookupResult, error) {
    // Try registry first
    result, err := c.lookupRemote(candidates)
    if err == nil {
        c.cache.Store(result)
        return result, nil
    }

    // Fall back to cache + bundled
    return c.lookupLocal(candidates)
}
```

## Registry Index Structure

For efficient lookup, registry builds inverted index:

```
signatures/
  index.json           # full inverted index
  by-manager/
    brew.json          # {package_name: [lore_packages]}
    apt.json
    cargo.json
    ...
  by-url-pattern.json  # [{pattern: regex, packages: []}]
```

Index entry:
```json
{
  "brew": {
    "ripgrep": ["ripgrep"],
    "rg": ["ripgrep"],
    "neovim": ["neovim"],
    "nvim": ["neovim"]
  },
  "apt": {
    "ripgrep": ["ripgrep"],
    "neovim": ["neovim"]
  }
}
```

## Implementation Phases

### Phase 1: Schema + Local
- Add `signatures` field to manifest schema
- Populate signatures for existing packages
- Build local signature index at `lore update` time
- Migrate tool uses local index

### Phase 2: Registry API
- Implement `/v1/signatures/lookup` endpoint
- Registry builds and serves signature index
- Migrate tool calls API with cache fallback

### Phase 3: Community Growth
- Accept signature contributions via PR
- Auto-detect signatures from manifest install commands
- Track "unresolved" reports to prioritize new packages

