# Follow-up actions

1. Receiver params registration. Should the code generator ensure that the params required by marshal.go are registered on the same code path as the providers. It strikes me as possible and desireable.

2. The code generator should ensure that variadic args present as an arrray to starlark. You can pass a slice in lieu of variadic args to methods like file.Provider.Join.

   See starlark.UnpackArgs. If a provider method has a variadic parameter, the value of that variadic parameter is the list of args without param names.

3. We need terminal flow control nodes: Complete => A successful execution (the default and standard, healthy conclusion of a path), Degraded (the graph continues, but marks the branch as non-optimal), and Fatal (the graph execution stops immediately due to a failure)

   - Complete accepts an output value that can be captured by graph consumer or nil.
   - Degraded accepets a warning message that maps to a go error, captured as a memory resource.
   - Fatal accepts a fail error message that maps to a go error, captured as a memory resourc.e

4. Remove all generated files from source control. `make clean` should remove them. `git` should ignore them. There is nothing to preserve. star is now definitive.
