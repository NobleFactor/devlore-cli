# Package Signatures

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
