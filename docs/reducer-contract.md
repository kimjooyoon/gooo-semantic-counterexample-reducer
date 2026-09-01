# Reducer contract

`meta/counterexample-reducer.gooo` is the authoritative semantic contract.

The runner performs the declared operations in order:

1. remove deterministic chunks of nodes, also removing incident edges;
2. remove deterministic chunks of edges;
3. clear deterministic chunks of graph, node, and edge payloads to minimize serialized bytes.

Every candidate is replayed twice by the declared oracle. A candidate is accepted only
when all preserved predicates match the baseline: decision, reason digest, source digest,
contract digest, and toolchain digest. The runner continues through decision-changing
candidates while searching, but the final report is `REFUTED` if the final candidate does
not preserve the baseline pair.

The fixture digest is checked before the baseline oracle is called. This makes a changed
fixture an explicit `UNKNOWN` instead of silently consuming mutable input.
