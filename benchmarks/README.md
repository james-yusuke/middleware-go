# Router benchmark lab

This module keeps third-party framework dependencies out of the core
`middleware-go` module. Versions are pinned in `go.mod` and all adapters are
covered by hit/miss correctness tests before their benchmarks are run.

## Comparison tiers

The result groups are intentionally separate. Numbers from different tiers
must not be placed in one ranking table.

| Tier | Benchmark | Routers | What is measured |
|---|---|---|---|
| Pure lookup | `BenchmarkPureLookup` | middleware-go Trie/Regex/Aho-Corasick, httprouter, Chi, Echo | Public route-lookup API, including the context reset/pool required by that API |
| Native HTTP | `BenchmarkNativeHTTPDispatch` | net/http ServeMux, middleware-go, httprouter, Chi, Gin, Echo | Each implementation's complete `http.Handler` dispatch path |
| Fiber bridge | `BenchmarkFiberProtocolBridge` | Fiber | `App.Test`, including net/http to fasthttp protocol conversion |
| Parallel | `*Parallel` | Pure and native HTTP sets | Concurrent read-only lookup/dispatch |
| Build | `BenchmarkBuildRoutes` | Pure lookup set | Router construction and batch insertion |

Fiber is isolated because `App.Test` serializes and parses an HTTP request; it
is not equivalent to calling an in-process `net/http.Handler`. Gin has no
public direct lookup API, so it appears only in native dispatch. These are API
boundaries, not omissions.

## Corpus

The deterministic corpus contains an even mixture of:

- static routes;
- one-parameter routes;
- suffix catch-all routes;
- two-parameter routes.

Each logical route is translated to the target framework's syntax. Workloads
are `hit`, `miss`, and `mixed` (80% hit). All adapters must pass the same
logical route checks in `adapters_test.go`.

The benchmark defaults are controlled by environment variables:

| Variable | Default | Example |
|---|---:|---|
| `ROUTER_BENCH_SIZES` | `100,1000,10000` | `100000,1000000` |
| `ROUTER_NATIVE_SIZES` | `100,1000` | `10000` |
| `ROUTER_PARALLEL_SIZES` | `1000` | `10000` |
| `ROUTER_BUILD_SIZES` | `100,1000` | `10000` |
| `ROUTER_FIBER_SIZES` | `100` | `1000` |
| `ROUTER_BENCH_ROUTERS` | all pure routers | `httprouter,chi,echo` |

Large-scale runs exclude the linear Regex implementation by default because
its expected runtime is proportional to the route count. It can still be
selected explicitly for controlled scaling experiments.

## Adding another router

Add its pinned module version, implement either `pureLookup` or a
`nativeFactory`, define syntax translation when necessary, and add it to the
adapter correctness test. Do not adapt a private internal API solely to make a
number look comparable; use a separate tier when the public execution boundary
is materially different.

For publishable measurements, keep the container image digest, CPU model,
Docker resource limits, `GOMAXPROCS`, benchmark count, and raw output. Compare
multiple samples with `benchstat`; a single `ns/op` result is only a smoke
check.
