# Queued: Distributed Coordination Analysis

## Question

A distributed systems coordinator should be able to target a host with a
deployment. Two options to analyze:

1. **Command forwarding**: Coordinator issues `lore deploy @manifest.yaml`
   to targeted hosts (perhaps many at once)
2. **Subgraph shipping**: Coordinator sends a subgraph to each targeted host
   and runs it locally

Analyze pros, cons, and alternatives for both options. Consider what we know
about the lore and writ command lines and the architecture.

## Status

Queued for analysis. Research completed (CLI structure, execution flow,
manifest formats, graph serialization). Analysis pending.
