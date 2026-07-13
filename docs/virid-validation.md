# VIRID prototype validation record

Validation date: 2026-07-14

Environment used for the initial run:

```text
OS: darwin
Architecture: arm64
CPU: Apple M4
Go: 1.25.3
```

This document records what the Go prototype establishes and, equally
importantly, what it currently falsifies.

## What is implemented

- finite-field univariate polynomial multiplication, division, evaluation,
  normalization, and GCD;
- route ideals whose constraint generators specialize to zero on success and
  to a field unit on failure;
- exact enumeration of the generators of a product of route ideals;
- request specialization and route-code recovery from the roots of the GCD;
- an oracle comparison in every `Explain` trace;
- explicit priority, specificity, stable route ID, and exact final verification;
- static, parameter, one-segment wildcard, suffix catch-all, and optional
  segments;
- host, method, and API-version constraints;
- immutable delta generations, tombstones, atomic snapshot publication, and
  compaction;
- a hard generator cap with complete linear fallback.

## Mapping from claims to tests

| Claim | Test |
|---|---|
| Common route-code factors survive polynomial GCD | `TestVIRIDPolynomialGCDRecoversCommonRouteCode` |
| Non-matches collapse to the unit ideal and a match remains as `z-k` | `TestVIRIDUnitCollapseAndRouteIDRecovery` |
| Multiple matching routes are all roots before priority resolution | `TestVIRIDPriorityAndMultipleAlgebraicRoots` |
| Optional, wildcard, catch-all, host, and version semantics | `TestVIRIDOptionalWildcardCatchAllHostAndVersion` |
| Basis overflow never drops a route | `TestVIRIDBasisLimitFallsBackWithoutChangingResults` |
| Delta visibility, tombstones, and physical compaction | `TestVIRIDDeltaDeleteAndCompaction` |
| Agreement with a separately implemented scan oracle | `TestVIRIDDifferentialAgainstIndependentLinearOracle` |
| Snapshot publication is race-free under the tested schedule | `TestVIRIDConcurrentReadersAndSnapshotWriters` plus `go test -race` |

## Algebra trace

Command:

```bash
go run ./cmd/virid-debug -path /users/42 -compact=true
```

For the built-in corpus, the observed values were:

```text
logical routes:                 7
optional-expanded variants:    8
delta basis generators:       42
compacted basis generators: 92160
non-zero after specialization: 576
GCD coefficients: [2147483644, 1]
decoded root code: 3
decoded logical route ID: 3
oracle agreement: true
```

Since the field prime is `2147483647`, the GCD is

```text
2147483644 + z = z - 3
```

and therefore directly identifies route code 3.

The same command with `-max-basis 100` publishes an exact fallback generation
and still selects route 3.

## Initial microbenchmark observation

Commands:

```bash
go test ./middleware -run '^$' -bench '^BenchmarkRouterContractFind$' -benchmem -count=3
go test ./middleware -run '^$' -bench '^BenchmarkVIRIDResearchModes$' -benchmem -count=3
```

Observed ranges from the initial machine:

| Case | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Trie contract | 158–183 | 236 | 3 |
| Regex contract | 379–434 | 244 | 3 |
| Aho-Corasick contract | 159–177 | 236 | 3 |
| VIRID compacted contract | 28,348–31,406 | 16,590 | 719 |
| VIRID delta generations | 4,014–4,472 | 1,265 | 72 |
| VIRID compiled product ideal | 21,025–23,599 | 12,745 | 548 |
| VIRID forced linear fallback | 231–244 | 224 | 3 |

These are diagnostic microbenchmarks, not publication-quality results. They do
show that the current exact generator-enumeration implementation is not a
performance improvement over Trie/Radix-style routing. On this small corpus,
the deliberately simple fallback is much faster than the algebraic evaluator.

## Current conclusion

The prototype supports the algebraic correctness claim for the covered finite
cases and makes false negatives observable through an oracle comparison. It
also provides direct evidence for the main threat identified in the report:
product-generator growth and allocation cost dominate lookup.

The next research gate is therefore not additional low-level SIMD work. It is
to find a certified generator compression or winner-recovery representation
that avoids enumerating the product ideal. Without that result, VIRID should
remain a mathematical validation experiment rather than a proposed production
router.

