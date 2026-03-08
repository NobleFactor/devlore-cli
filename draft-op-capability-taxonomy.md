Your current structure—`op/provider/<domain>`—is a solid foundation, but the friction you are feeling with `callable` and `encryption` suggests you are conflating **"Domain of Authority"** (who owns the resource) with **"Operational Capability"** (what the resource can do).

In system architecture, when a domain feels "awkward," it’s usually because it is a **Cross-Cutting Concern** masquerading as a **Vertical Domain**.

### 1. The Strategy: Separate "Origins" from "Traits"

Stop treating every directory in `provider` as an equal domain. Instead, distinguish between:

* **Origin Domains (The "Where"):** `git`, `file`, `mem`, `service`. These represent *where* the data lives.
* **Trait/Capability Domains (The "What"):** `callable`, `encryption`, `schema`. These represent *how* the data behaves, regardless of its origin.

### 2. Proposed Restructure: The "Trait-Based" Model

Move your "oddball" domains into a specific hierarchy that signals they are modifiers, not data sources.

```text
op/
  provider/
    origin/       # Where data lives
      git/
      file/
      mem/
      appnet/
    trait/        # Capabilities that wrap/modify resources
      callable/   # A trait that allows execution
      encrypt/    # A trait that adds security
      validate/   # A trait that enforces schema

```

### 3. Solving the "Callable" Awkwardness

A `callable` isn't a domain—it's a **Functional Trait**. By moving it into a `trait` namespace, you change your mental model:

* **Before:** I have a `callable.Resource`. (Feels like a separate bucket of things).
* **After:** I have a `file.Resource` that has been **upgraded** with the `callable` trait.

This allows you to implement a **Decorator Pattern** or **Mix-in Architecture**. Your graph engine doesn't need to know if a resource is `mem` or `git` if it just checks for the `Callable` interface.

### 4. Handling Cross-Domain Operations (The "Encryption" Case)

The fact that `encryption` operates on `file` and `mem` proves it is a **Coordinator Domain**.

Don't put `encryption` inside `provider/`. Create a separate `op/security/` or `op/transform/` package.

* **`op/provider`**: Should *only* contain things that fetch/produce base resources.
* **`op/transform`**: Should contain things that take a `Resource` and return a new `Resource` (e.g., `Encrypt(file.Resource) -> EncryptedResource`).

### Why this fixes the "Awkward" feeling:

1. **Semantic Clarity:** When a developer sees `op/provider/origin/git`, they know exactly what it does. When they see `op/provider/trait/callable`, they know it's a behavior wrapper.
2. **Scalability:** If you add a new provider (e.g., `s3`), you only touch `origin/`. If you add a new capability (e.g., `compress`), you only touch `trait/`.
3. **Graph Construction:** Your graph engine can now plan actions based on **Capabilities**. A plan can now say: *"I need a `Callable` resource,"* and the system can find a `file.Resource` and satisfy the request by wrapping it in the `callable` trait.

### Summary of the New Hierarchy

* **`op/provider/origin/`**: The "leaves" of your graph (data sources).
* **`op/provider/trait/`**: The "logic" of your graph (execution, crypto, validation).
* **`op/transform/`**: The "glue" (the functions that bridge origins and traits).

This separation prevents you from polluting your "domains" with cross-cutting logic.

Does this "Trait vs. Origin" distinction help resolve why `callable` felt like a misplaced domain, or does it complicate your current codegen setup too much?
