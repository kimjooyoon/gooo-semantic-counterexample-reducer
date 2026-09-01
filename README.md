# Gooo semantic counterexample reducer

This repository contains an executable reducer for failed Gooo semantic graphs. It turns
the declared graph into an explainable, smaller counterexample while preserving the
original oracle decision and exact reason digest.

The reducer is driven by [`meta/counterexample-reducer.gooo`](meta/counterexample-reducer.gooo).
That metacode owns the denominator, oracle predicates, reduction operation order,
generation plan, status precedence, and the two pinned representative scenarios:

- the evolution-trial four-activity rejection;
- name capture followed by privilege escalation.

Go is limited to parsing the metacode, executing its declared runner, and emitting JSON
artifacts. Fixtures are consumed only after their declared immutable digest matches.
Generated output is written to a caller-owned directory outside the input repository.

## Status model

`CLOSED` means the final graph preserves the baseline decision and reason digest.
`UNKNOWN` means the oracle or input identity was not stable; it always includes `stage`,
`step`, `reason`, `unknown_class`, `next_operation`, and `blocked_by`. `REFUTED` means
the final candidate changed the decision or reason. Precedence is `REFUTED > UNKNOWN > CLOSED`.

An improvement claim is `UNKNOWN` unless a prior report matches the same scenario,
source digest, contract digest, and toolchain digest. Exact node, edge, byte, and oracle
invocation integers are emitted for every scenario.

## CI-only verification

The repository was bootstrapped from `gooo-repository-bootstrap v0.1.1`. The bootstrap
commit is the only direct-main exception; subsequent changes are delivered through a PR.
GitHub Actions is the verification authority. It runs Go 1.27, records exact inventory,
runtime, memory, test, and generated-artifact metrics, checks that the input repository
is unchanged, and uploads the complete reduction evidence. Local test, conformance, or
execution results are not used as a success claim.

The release workflow is manually dispatched only for the exact merged main SHA after CI
passes. It creates one annotated tag and an immutable GitHub release containing the
source archive, reports, metrics, manifest, and SHA256 sums.
