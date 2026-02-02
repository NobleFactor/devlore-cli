# E2E Testing Architecture

This document describes the end-to-end testing strategy for LLM-integrated
commands (`writ migrate` and `lore onboard`), including multi-provider testing,
performance metrics, and correctness evaluation.

## Design Goals

1. **Multi-Provider Coverage**: Test with Ollama, Anthropic, GitHub Models, OpenAI
2. **Reproducible Results**: Fixtures produce consistent expected outputs
3. **Performance Tracking**: Measure latency, token usage, and cost
4. **Correctness Metrics**: Precision, recall, F1 for structured outputs
5. **CI Integration**: Run subset in CI, full suite on-demand

## Test Infrastructure

```
internal/e2e/
├── harness.go          # Test harness, metrics, reporting
├── migrate_test.go     # writ migrate E2E tests
├── onboard_test.go     # lore onboard E2E tests
└── testdata/
    ├── migrate/
    │   ├── tuckr-style/
    │   ├── stow-style/
    │   ├── chezmoi-style/
    │   └── script-based/
    └── onboard/
        ├── ripgrep.html
        ├── ripgrep-expected.yaml
        ├── nodejs.html
        └── nodejs-expected.yaml
```

## Test Harness

### Provider Configuration

```go
type ProviderConfig struct {
    Name     string            // e.g., "ollama", "anthropic"
    Provider model.Provider    // Actual provider instance
    Model    string            // Model name for this provider
    Enabled  bool              // Whether to run tests with this provider
}

func DefaultProviders() []ProviderConfig {
    return []ProviderConfig{
        {
            Name:    "ollama",
            Model:   "llama3.2",
            Enabled: isOllamaRunning(),
        },
        {
            Name:    "anthropic",
            Model:   "claude-3-5-sonnet-20241022",
            Enabled: os.Getenv("ANTHROPIC_API_KEY") != "",
        },
        {
            Name:    "github",
            Model:   "gpt-4o",
            Enabled: os.Getenv("GITHUB_TOKEN") != "",
        },
    }
}
```

### Metrics Collection

```go
type PerformanceMetrics struct {
    LatencyMs    int64   `json:"latency_ms"`
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    TotalTokens  int     `json:"total_tokens"`
    CostUSD      float64 `json:"cost_usd,omitempty"`
    Retries      int     `json:"retries"`
}

type CorrectnessMetrics struct {
    TotalExpected  int     `json:"total_expected"`
    TotalFound     int     `json:"total_found"`
    TruePositives  int     `json:"true_positives"`
    FalsePositives int     `json:"false_positives"`
    FalseNegatives int     `json:"false_negatives"`
    Precision      float64 `json:"precision"`
    Recall         float64 `json:"recall"`
    F1Score        float64 `json:"f1_score"`

    // Domain-specific flags
    SystemCorrect  bool `json:"system_correct,omitempty"`  // For migration
    ProductCorrect bool `json:"product_correct,omitempty"` // For onboarding
}
```

### Test Result Structure

```go
type TestResult struct {
    Name        string             `json:"name"`
    Provider    string             `json:"provider"`
    Model       string             `json:"model"`
    Passed      bool               `json:"passed"`
    Error       string             `json:"error,omitempty"`
    Performance PerformanceMetrics `json:"performance"`
    Correctness CorrectnessMetrics `json:"correctness"`
    Timestamp   time.Time          `json:"timestamp"`
}
```

## Fixtures

### Migration Fixtures

Each fixture represents a different dotfile management system:

**`tuckr-style/`** — Tuckr repository structure:
```
tuckr-style/
├── expected.yaml           # Expected analysis output
├── Configs/
│   ├── all/
│   │   └── .zshrc
│   ├── all_darwin/
│   │   └── .config/alacritty/
│   └── all_linux/
│       └── .config/systemd/
├── Hooks/
│   └── post_all.sh
└── Install-Configuration.sh  # Contains "tuckr add" commands
```

**`expected.yaml`**:
```yaml
system: tuckr
system_confidence_min: 0.9
structure:
  groups_path: Configs
  naming_convention: "<group>_{platform}"
required_renames:
  - from: Configs/all_darwin
    to: Configs/all.Darwin
  - from: Configs/all_linux
    to: Configs/all.Linux
```

**`stow-style/`** — GNU Stow structure:
```
stow-style/
├── expected.yaml
├── .stow-local-ignore
├── bash/
│   ├── .bashrc
│   └── .bash_profile
├── vim/
│   ├── .vimrc
│   └── .vim/
└── install.sh              # Contains "stow -t ~" commands
```

**`chezmoi-style/`** — chezmoi structure:
```
chezmoi-style/
├── expected.yaml
├── .chezmoiignore
├── dot_zshrc
├── dot_config/
│   └── private_git/
│       └── config
├── run_once_install-packages.sh
└── executable_dot_local/
    └── bin/
        └── my-script
```

### Onboarding Fixtures

HTML pages with expected extraction results:

**`ripgrep.html`** — Simple CLI tool:
```html
<!DOCTYPE html>
<html>
<head><title>ripgrep</title></head>
<body>
  <h1>ripgrep (rg)</h1>
  <p>A line-oriented search tool that recursively searches directories.</p>

  <h2>Installation</h2>
  <h3>macOS</h3>
  <pre>brew install ripgrep</pre>

  <h3>Ubuntu/Debian</h3>
  <pre>sudo apt install ripgrep</pre>

  <h3>Cargo</h3>
  <pre>cargo install ripgrep</pre>
</body>
</html>
```

**`ripgrep-expected.yaml`**:
```yaml
product: ripgrep
aliases: [rg]
description_contains: "search"
category: cli_tool
installation:
  darwin:
    source: brew
    package: ripgrep
  debian:
    source: apt
    package: ripgrep
  cargo:
    source: cargo
    package: ripgrep
```

## Correctness Evaluation

### Migration Correctness

```go
func EvaluateMigrationCorrectness(actual *MigrationAnalysis,
    expected *ExpectedMigration) CorrectnessMetrics {

    metrics := CorrectnessMetrics{}

    // System detection
    metrics.SystemCorrect = strings.EqualFold(actual.System, expected.System)

    // Rename operations (set comparison)
    expectedRenames := toSet(expected.RequiredRenames)
    actualRenames := toSet(extractRenames(actual.ExecutionGraph))

    metrics.TotalExpected = len(expectedRenames)
    metrics.TotalFound = len(actualRenames)

    for rename := range actualRenames {
        if expectedRenames[rename] {
            metrics.TruePositives++
        } else {
            metrics.FalsePositives++
        }
    }
    metrics.FalseNegatives = metrics.TotalExpected - metrics.TruePositives

    // Calculate precision/recall/F1
    if metrics.TotalFound > 0 {
        metrics.Precision = float64(metrics.TruePositives) / float64(metrics.TotalFound)
    }
    if metrics.TotalExpected > 0 {
        metrics.Recall = float64(metrics.TruePositives) / float64(metrics.TotalExpected)
    }
    if metrics.Precision+metrics.Recall > 0 {
        metrics.F1Score = 2 * (metrics.Precision * metrics.Recall) /
            (metrics.Precision + metrics.Recall)
    }

    return metrics
}
```

### Onboarding Correctness

```go
func EvaluateOnboardingCorrectness(actual *OnboardResult,
    expected *ExpectedOnboard) CorrectnessMetrics {

    metrics := CorrectnessMetrics{}

    // Product identification
    metrics.ProductCorrect = strings.EqualFold(actual.Product, expected.Product)

    // Installation method detection
    for platform, install := range expected.Installation {
        if actualInstall, ok := actual.Installation[platform]; ok {
            if actualInstall.Source == install.Source {
                metrics.TruePositives++
            } else {
                metrics.FalsePositives++
            }
        } else {
            metrics.FalseNegatives++
        }
    }

    metrics.TotalExpected = len(expected.Installation)
    metrics.TotalFound = len(actual.Installation)

    // Calculate metrics...
    return metrics
}
```

## Test Execution

### Running Tests

```bash
# Run with default providers (Ollama only if running)
go test ./internal/e2e/... -v

# Run with specific provider
ANTHROPIC_API_KEY=sk-... go test ./internal/e2e/... -v -run TestMigrate

# Run with all available providers
ANTHROPIC_API_KEY=sk-... GITHUB_TOKEN=$(gh auth token) \
    go test ./internal/e2e/... -v

# Generate report
go test ./internal/e2e/... -v -json | go run ./cmd/e2e-report > report.md
```

### Test Functions

```go
func TestMigrateTuckrStyle(t *testing.T) {
    suite := e2e.NewTestSuite(t)

    for _, provider := range suite.EnabledProviders() {
        t.Run(provider.Name, func(t *testing.T) {
            // Load fixture
            fixture := suite.LoadMigrationFixture("tuckr-style")

            // Run migration analysis
            start := time.Now()
            result, err := migrate.AnalyzeWithLLM(
                context.Background(),
                provider.Provider,
                fixture.Input,
            )
            elapsed := time.Since(start)

            // Record result
            testResult := &e2e.TestResult{
                Name:     "migrate/tuckr-style",
                Provider: provider.Name,
                Model:    provider.Model,
            }

            if err != nil {
                testResult.Passed = false
                testResult.Error = err.Error()
            } else {
                testResult.Correctness = e2e.EvaluateMigrationCorrectness(
                    result.Analysis, fixture.Expected)
                testResult.Passed = testResult.Correctness.F1Score >= 0.8
            }

            testResult.Performance = e2e.PerformanceMetrics{
                LatencyMs: elapsed.Milliseconds(),
                // Token counts from provider response
            }

            suite.RecordResult(testResult)
        })
    }
}
```

## Reporting

### Report Generation

```go
type TestReport struct {
    Timestamp time.Time     `json:"timestamp"`
    Platform  string        `json:"platform"`
    Results   []TestResult  `json:"results"`
    Summary   ReportSummary `json:"summary"`
}

type ReportSummary struct {
    TotalTests    int                       `json:"total_tests"`
    PassedTests   int                       `json:"passed_tests"`
    FailedTests   int                       `json:"failed_tests"`
    ByProvider    map[string]ProviderStats  `json:"by_provider"`
    AvgLatencyMs  map[string]int64          `json:"avg_latency_ms"`
    AvgF1Score    map[string]float64        `json:"avg_f1_score"`
}
```

### Markdown Output

```markdown
# E2E Test Report

**Date:** 2025-01-30T10:00:00Z
**Platform:** darwin/arm64

## Summary

| Provider | Tests | Passed | Failed | Avg Latency | Avg F1 |
|----------|-------|--------|--------|-------------|--------|
| ollama   | 8     | 7      | 1      | 2340ms      | 0.92   |
| anthropic| 8     | 8      | 0      | 1850ms      | 0.98   |

## Results by Test

### migrate/tuckr-style

| Provider | Passed | System | Precision | Recall | F1    | Latency |
|----------|--------|--------|-----------|--------|-------|---------|
| ollama   | ✓      | ✓      | 1.00      | 1.00   | 1.00  | 2100ms  |
| anthropic| ✓      | ✓      | 1.00      | 1.00   | 1.00  | 1650ms  |

### migrate/chezmoi-style

| Provider | Passed | System | Precision | Recall | F1    | Latency |
|----------|--------|--------|-----------|--------|-------|---------|
| ollama   | ✗      | ✓      | 0.80      | 0.67   | 0.73  | 2580ms  |
| anthropic| ✓      | ✓      | 1.00      | 0.89   | 0.94  | 2010ms  |
```

## CI Integration

### GitHub Actions Workflow

```yaml
name: E2E Tests

on:
  schedule:
    - cron: '0 6 * * *'  # Daily at 6 AM UTC
  workflow_dispatch:

jobs:
  e2e-ollama:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Ollama
        run: curl -fsSL https://ollama.com/install.sh | sh

      - name: Start Ollama
        run: ollama serve &

      - name: Pull model
        run: ollama pull llama3.2

      - name: Run E2E tests
        run: go test ./internal/e2e/... -v -timeout 30m

  e2e-cloud:
    runs-on: ubuntu-latest
    if: github.event_name == 'workflow_dispatch'
    environment: e2e-testing  # Protected environment with secrets
    steps:
      - uses: actions/checkout@v4

      - name: Run E2E tests with cloud providers
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: go test ./internal/e2e/... -v -timeout 30m
```

### Fork Safety

```go
func (s *TestSuite) EnabledProviders() []ProviderConfig {
    // In CI, only enable providers with available credentials
    // Fork PRs won't have access to secrets

    if os.Getenv("CI") == "true" && os.Getenv("GITHUB_EVENT_NAME") == "pull_request" {
        // Only Ollama for PR checks (no secrets needed)
        return []ProviderConfig{
            {Name: "ollama", Model: "llama3.2", Enabled: true},
        }
    }

    return s.allProviders
}
```

## Best Practices

### Fixture Design

1. **Minimal but realistic** — Include enough structure to test detection, not full repos
2. **Platform coverage** — Each fixture should have platform-specific variants
3. **Edge cases** — Include unusual patterns (encrypted files, symlinks, templates)
4. **Expected values** — Document expected outputs with confidence thresholds

### Metric Thresholds

| Metric | Minimum for Pass | Target |
|--------|------------------|--------|
| F1 Score | 0.80 | 0.95 |
| System Detection | 100% | 100% |
| Precision | 0.85 | 1.00 |
| Recall | 0.75 | 0.95 |

### Debugging Failures

1. **Save raw outputs** — Log LLM responses for failed tests
2. **Diff expected vs actual** — Show specific differences
3. **Retry with verbose** — Enable provider-level logging
4. **Compare across providers** — Same fixture may pass on one, fail on another
