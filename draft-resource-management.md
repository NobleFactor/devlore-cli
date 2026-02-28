This design addresses the "Lineage Problem" in infrastructure-as-code: how to ensure that a single path (like `/etc/foo`) can be referenced, modified, and replaced within a single execution graph without ambiguity or race conditions.

### Architectural Summary

The system separates **Intent** (Planning) from **Reality** (Execution) using three core components:

1. **The Resource List (Ledger):** A flat, versioned history of every state a resource takes. Each entry is a `Resource[T]` with a unique ID.
2. **The Namespace Map (Lens):** A mutable lookup table used during the `Plan` phase. It maps a URI (e.g., `file:///etc/nginx.conf`) to the *latest* Resource ID in the script's flow.
3. **Shadowing:** When a plan method (like `file.Write`) executes, it creates a *new* Resource ID and updates the Namespace Map. Subsequent calls for that URI receive the new ID, creating an implicit dependency chain.

---

### Implementation Code

#### 1. Core Resource & Manager

The `Resource[T]` is a type-safe handle. The `ResourceManager` handles the "Discovery" vs "Promise" logic.

```go
// Resource is the handle passed between Starlark and Go Plan methods.
type Resource[T any] struct {
	ID           string // Unique ID (e.g., res_8f2b1a)
	URI          string // The logical address (e.g., file:///etc/hosts)
	OriginNodeID string // The Node that produces this (empty if pre-existing)
}

type Metadata map[string]any

type ResourceManager struct {
	mu       sync.Mutex
	Ledger   []any            // Flat list of Resource[T]
	Metadata map[string]Metadata // Discovery data (Inodes, Hashes)
}

// EnsureCataloged is the "Registration Desk"
func (m *ResourceManager) EnsureCataloged[T any](uri string, producerID string) Resource[T] {
	m.mu.Lock()
	defer m.mu.Unlock()

	res := Resource[T]{
		ID:           fmt.Sprintf("res_%d", len(m.Ledger)),
		URI:          uri,
		OriginNodeID: producerID,
	}
	m.Ledger = append(m.Ledger, res)
	return res
}

```

---

#### 2. Specific Resource Types (Blobs, Git, Literals, Services)

```go
// Blob represents a file-like resource
type Blob struct {
	Path   string
	Hash   string
	Size   int64
	Inode  uint64
}

// GitState represents a repository at a specific commit
type GitState struct {
	URL    string
	Commit string
}

// ServiceState represents the status of a system daemon
type ServiceState struct {
	Name   string
	Status string // "running", "stopped"
}

// Literal represents in-memory data (String, JSON, YAML, Bytes)
type Literal[T any] struct {
	Value T
}

```

---

#### 3. The Graph Builder & Namespace Map

This is where **Shadowing** occurs. The `Builder` maintains the "Current" version of every URI.

```go
type GraphBuilder struct {
	Manager   *ResourceManager
	Namespace map[string]string // URI -> ResourceID
}

// Resolve looks up the "Current" version or catalogs a new discovery
func (gb *GraphBuilder) Resolve[T any](uri string) Resource[T] {
	if id, exists := gb.Namespace[uri]; exists {
		// Return the existing resource from the ledger
		// (In practice, you'd look this up in the flat list)
		return Resource[T]{ID: id, URI: uri}
	}
	
	// First encounter: Catalog as a "Pre-existing" resource
	res := gb.Manager.EnsureCataloged[T](uri, "")
	gb.Namespace[uri] = res.ID
	return res
}

// Shadow updates the namespace to point to a new version (Promise)
func (gb *GraphBuilder) Shadow[T any](uri string, producerNodeID string) Resource[T] {
	res := gb.Manager.EnsureCataloged[T](uri, producerNodeID)
	gb.Namespace[uri] = res.ID
	return res
}

```

---

### 4. Example: Plan Method Calls

Here is how a Provider uses these tools to ensure resources are cataloged correctly and dependencies are wired.

```go
type FileProvider struct {
	Builder *GraphBuilder
}

// Example 1: file.Copy(src, dest)
func (p *FileProvider) Copy(srcPath, destPath string) (Resource[Blob], error) {
	srcURI := "file://" + srcPath
	destURI := "file://" + destPath

	// 1. Resolve the Source (Might be a previous node's output)
	srcRes := p.Builder.Resolve[Blob](srcURI)

	// 2. Create the Node in the graph
	nodeID := "node_" + uuid.NewString()

	// 3. Shadow the Destination (This node NOW owns this path)
	destRes := p.Builder.Shadow[Blob](destURI, nodeID)

	fmt.Printf("PLAN: Copy %s (ID:%s) to %s (ID:%s)\n", srcURI, srcRes.ID, destURI, destRes.ID)
	return destRes, nil
}

// Example 2: pkg.Install(name) -> involves shadowing a service or binary path
func (p *FileProvider) WriteJSON(path string, data any) (Resource[Blob], error) {
	uri := "file://" + path
	
	// Catalog a LiteralResource for the JSON content
	litRes := p.Builder.Manager.EnsureCataloged[Literal[any]]("mem://json_payload", "")

	nodeID := "node_" + uuid.NewString()
	
	// Shadow the file path with the promise of this write
	fileRes := p.Builder.Shadow[Blob](uri, nodeID)
	
	return fileRes, nil
}

// Example 3: git.Clone(url, path)
func (p *FileProvider) GitClone(repoURL, targetPath string) (Resource[GitState], error) {
    nodeID := "node_" + uuid.NewString()
    
    // Shadow the path: any future file.Read on targetPath must wait for GitClone
    p.Builder.Shadow[Blob]("file://"+targetPath, nodeID)
    
    // Catalog and return the GitResource state
    return p.Builder.Shadow[GitState]("git://"+repoURL, nodeID), nil
}

```

---

### Key Takeaways for the Design Doc

* **Decoupling:** The Plan function never touches the disk. It only manipulates the `Namespace Map` and `Resource List`.
* **Implicit Versioning:** By calling `Shadow`, the provider ensures that if the Starlark script calls `file.Read("/etc/foo")` twice—once before and once after a `file.Write`—the two calls will resolve to different `Resource IDs`.
* **URI as Key:** Using URI schemes (`file://`, `git://`, `mem://`) allows the `Namespace Map` to act as a unified conflict-resolver across all resource types.

### The Executor: Resolving Intent into Physical State

While the **Planner** lives in a world of strings and potential, the **Executor** must translate those URI-based "Promises" into actual filesystem operations. The core challenge is handling the **Shadowing** event: when the graph dictates that a new resource (`res_C3`) should occupy a path currently held by an older resource (`res_A1`).

#### 1. The Conflict Analysis (Pre-Flight)

Before the first node executes, the Executor performs a "Binding" pass. It compares the **Namespace Map** (the final desired state) against the **Initial Discovery** (what is currently on disk).

If a URI (e.g., `file:///etc/foo`) is associated with `Resource_A` in Discovery but will be produced as `Resource_C` by the Graph, the Executor marks `Resource_A` for a **Tombstone**.

#### 2. The Tombstone Implementation

A Tombstone is a temporary "safety-deposit box" for the shadowed resource. It ensures that if the node producing the new version fails, the system can revert to the exact Inode and Hash that existed previously.

```go
type Tombstone struct {
	OriginalID   string    // e.g., res_A1
	BackupPath   string    // Path in the local .blobs/ storage
	OriginalURI  string    // e.g., file:///etc/foo
	Metadata     Metadata  // The Inode, Hash, and Permissions
}

// PrepareTombstone captures the state before an overwrite
func (e *Executor) PrepareTombstone(res Resource[Blob]) (*Tombstone, error) {
	// 1. Capture the current physical state
	info, _ := os.Lstat(res.URI)
	
	// 2. Move the file to a content-addressed storage (CAS) area
	backupPath := fmt.Sprintf(".op/blobs/%s", res.Metadata["hash"])
	err := os.Rename(res.URI, backupPath) 
	
	return &Tombstone{
		OriginalID:  res.ID,
		BackupPath:  backupPath,
		OriginalURI: res.URI,
		Metadata:    res.Metadata,
	}, err
}

```

---

### 3. The Execution Loop: Resolving "Wait-Groups"

The Executor uses a **Reactive Slot** model. Each `Resource[T]` starts in a `Pending` state. Nodes stay blocked until their input Slots are transitioned to `Fulfilled`.

```go
type Slot struct {
	ID        string
	Value     any        // The actual Blob, GitState, or Literal
	Fulfilled chan struct{} // Closed when the producer node finishes
}

func (e *Executor) RunNode(n Node) {
	// 1. Wait for all inputs to be ready
	for _, inputID := range n.InputIDs {
		slot := e.GetSlot(inputID)
		<-slot.Fulfilled // Block until producer finishes
	}

	// 2. Execute the physical action (e.g., file.Write)
	result, err := n.Action.Execute()

	// 3. Fulfill the output slot
	outputSlot := e.GetSlot(n.OutputID)
	outputSlot.Value = result
	close(outputSlot.Fulfilled)
	
	// 4. Update the live Namespace Map for dynamic lookups
	e.Namespace[n.URI] = n.OutputID
}

```

---

### 4. Summary of the Lifecycle

1. **Starlark Plan:** User defines `file.write("/etc/foo")`. This **Shadows** the URI in the `NamespaceMap`, creating a new `ResourceID` (the Promise).
2. **Graph Transport:** The list of `ResourceIDs` and their `URI` associations are sent to Box B.
3. **Executor Binding:** Box B sees that `/etc/foo` is currently `Inode 123` but is promised to become `Inode 456`. It creates a **Tombstone** for `Inode 123`.
4. **Reactive Execution:** The `Write` node executes. It fulfills the **Slot** for the new `ResourceID`. Any downstream nodes waiting for `/etc/foo` now receive the new Inode and Hash.

