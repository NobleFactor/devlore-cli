# The Projected Provider API

This architecture creates a very clean separation between the "Definition" of a capability and its "Manifestation" in the developer's script. By using the **Immediate** vs. **Planned** language, we move away from technical jargon (e.g., eager/graph) and toward a user-centric model of **Execution Intent**.

Here is the summary of the subscription and registration model for the Projected Provider API:

---

## 1. The Provider Source of Truth

The foundation is a hand-coded Go struct (the **Provider**). It serves as a single source of truth for all logic. You use **doc-comment directives** (Pragmas) to define how each method should be projected into the Starlark environment.

- **`//devlore:access=immediate`**: Fact-finding or meta-actions (e.g., `exists`, `note`).
- **`//devlore:access=planned`**: State-changing actions that belong in the DAG (e.g., `install`, `symlink`).
- **Default (`both`)**: Versatile methods that can return a value now or a Node handle later.

## 2. The Registration Process

Consumers (your specific dev-box tools or runners) do not blindly import all code. They explicitly **register** the providers they need. This acts as a "Subscription" to the provider's capabilities.

During registration, the consumer defines the **scope of usage**:

- **Register for Immediate**: The methods are bound to the **Global/Package namespace**.
- **Register for Planned**: The methods are bound to the **`plan` root object**.

## 3. The Subscription Lifecycle

1. **Hand-Coding**: You write the Go logic once in the Provider.
2. **Generation**: Your tool parses the doc-comments and generates two distinct "Receivers" (wrappers):

- **Immediate Receiver**: Executes the Go method and returns the result to Starlark.
- **Planned Receiver**: Wraps the call into a `Task` or `Action` and returns a reference for the execution graph.

3. **Binding**: At runtime, the Registry maps these generated receivers to the Starlark identifiers based on the consumer's registration.

---

## 4. The User Experience

The result is a highly intuitive Starlark API where the prefix dictates the behavior of the same underlying provider logic:

| Action Type   | Syntax Example           | When it happens          | Starlark Context                   |
| ------------- | ------------------------ | ------------------------ | ---------------------------------- |
| **Immediate** | `pkg.is_installed("go")` | During script evaluation | Used for control flow (`if/else`). |
| **Planned**   | `plan.pkg.install("go")` | During graph execution   | Used to build the target state.    |

---

### Key Strengths of this Model

- **No Code Duplication**: One Go method powers two different Starlark experiences.
- **Safety by Design**: A provider method meant only for "Pre-flight" (Immediate) cannot accidentally be put into the "Execution" (Planned) graph if the generator enforces the doc-comment tags.
- **Scalable Discovery**: Users look at the `plan.` namespace to see everything that affects the box's final state, and the global namespace for everything else.

# To wrap this all up, let’s look at how a user would actually experience the **Projected Provider API**.

Since the logic is hand-coded once but "projected" into two distinct namespaces, the documentation needs to explain that while the _functionality_ is the same, the _timing_ and _return values_ differ.

---

# The Projected Provider API: A User Guide

In this system, providers (like `file`, `pkg`, or `git`) exist in two dimensions. You choose the dimension based on whether you need a result **now** to make a decision, or a result **later** to build your box.

### 1. The Immediate Projection (Global)

Methods called directly on the package name are executed **immediately** during the script evaluation. Use these for facts, logic, and pre-flight checks.

- **Namespace:** `pkg_name.action()`
- **Returns:** The actual data (string, bool, int).
- **Use Case:** "If the user is on macOS, then..."

```python
# Immediate: returns a boolean right now
if host.is_macos():
    note("Setting up Mac specific tools...")

```

### 2. The Planned Projection (`plan` root)

Methods called under the `plan` object are **deferred**. They are added to the execution graph and will be resolved in the optimal order during the convergence phase.

- **Namespace:** `plan.pkg_name.action()`
- **Returns:** A `Task` handle (used for dependencies).
- **Use Case:** "Ensure this package is installed eventually."

```python
# Planned: returns a Task handle for the graph
go_task = plan.pkg.install("golang")

# We can use the handle to enforce order
plan.file.copy("env.sh", dest="/etc/profile.d/", after=go_task)

```

---

## Developer Reference: The Registration Flow

When you add a new Provider to the ecosystem, the lifecycle follows this "Projected" path:

1. **Define:** You write a Go struct and tag it:

```go
//devlore:access=planned
func (p *Provider) Install(ctx Context, name string) { ... }

```

2. **Generate:** The tooling creates the **Immediate** and **Planned** receivers.
3. **Subscribe:** The consumer binary registers the provider:

```go
registry.Bind("pkg", pkg.Provider, registry.Immediate(), registry.Planned())

```

4. **Execute:** The Starlark interpreter sees two distinct ways to call `install()`, handled by the generated shims.

### Why this works for Dev Boxes

This model prevents the "Double-Check" bug. In many systems, you have to write code to check if a file exists, and then write code to create it. Here, you use the **Immediate** projection to check state and the **Planned** projection to define the desired state.
