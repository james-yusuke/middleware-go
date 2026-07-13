# middleware-go

A simple and extensible HTTP middleware engine for Go, inspired by the elegant Gin style syntax.

## Features

- Minimal core, easy to read and extend
- Gin-like handler registration and middleware style
- Detailed logging and debug mode support
- Custom handler and middleware chaining
- No third-party dependencies
- Experimental VIRID (Vanishing-Ideal Route Identification and Decoding) router

## Reproducible router benchmark lab

The repository also acts as a version-pinned comparison lab for Go HTTP
routers. Third-party dependencies live in the nested `benchmarks` module, so
the middleware library itself remains dependency-free.

The current adapters cover:

- Go `net/http.ServeMux`
- middleware-go Trie, Regex, Aho-Corasick, and the separate VIRID research suite
- `httprouter`
- Chi
- Gin
- Echo
- Fiber

Build the common Go toolchain image once, then invoke a single Compose service
at a time. Running benchmark services concurrently would introduce scheduler
and cache interference.

```bash
docker compose build
docker compose run --rm test
docker compose run --rm race
docker compose run --rm benchmark-smoke
```

The main benchmark services are:

```bash
# Public direct-lookup APIs, with hit/mixed/miss workloads.
docker compose run --rm benchmark-pure

# Complete net/http dispatch paths.
docker compose run --rm benchmark-native

# Concurrent lookup and dispatch.
GOMAXPROCS=4 BENCH_CPUS=4 docker compose run --rm benchmark-parallel

# Construction and insertion throughput.
docker compose run --rm benchmark-build

# VIRID's compiled, delta-generation, and exact-fallback modes.
docker compose run --rm benchmark-virid
```

Route counts, duration, sample count, and router selection are configurable:

```bash
ROUTER_BENCH_SIZES=100,1000,10000 \
BENCHTIME=2s BENCHCOUNT=5 \
docker compose run --rm benchmark-pure

# The same service can exercise the 100k and 1M research points.
ROUTER_SCALE_SIZES=100000,1000000 \
BENCHTIME=500ms BENCHCOUNT=3 \
docker compose run --rm benchmark-scale
```

CPU and heap profiles are written to `artifacts/cpu.out` and
`artifacts/mem.out`; the matching symbolized binary is
`artifacts/benchmarks.test`:

```bash
ROUTER_PROFILE_ROUTER=project-trie ROUTER_PROFILE_SIZE=10000 \
docker compose run --rm benchmark-profile

docker compose run --rm benchmark-profile \
  pprof -top /workspace/artifacts/benchmarks.test \
  /workspace/artifacts/cpu.out
```

Pure lookup, native `net/http` dispatch, and Fiber's `App.Test` bridge are kept
in distinct result groups because their execution boundaries are not
equivalent. See [`benchmarks/README.md`](benchmarks/README.md) for the corpus,
fairness rules, environment variables, and adapter extension guide.

## Getting Started

### Installation

Clone this repository and use it in your own Go project:

```bash
git clone https://github.com/james-yusuke/middleware-go.git
```

Or simply copy the `middleware/` folder into your project.

### Quick Example

```go
package main

import (
    "middleware-go/middleware"
    "net/http"
    "time"
)

func main() {
    e := middleware.New()
    e.Use(middleware.Recover())
    e.Use(middleware.LimitBody(1 << 20))
    e.Use(middleware.Logger("srv"))
    e.Use(middleware.Timeout(5 * time.Second))

    _ = e.SwitchRouter(middleware.ModeTrie)

    e.GET("/ping", func(c *middleware.Context) {
        c.String(200, "pong")
    })
    e.GET("/users/:id", func(c *middleware.Context) {
        id := c.Param("id")
        c.JSON(200, map[string]string{"id": id})
    })

    api := e.Group("/api")
    api.GET("/items", func(c *middleware.Context) {
        c.String(200, "list")
    })

    http.ListenAndServe(":8080", e)
}
```

You can now access [http://localhost:8080/ping](http://localhost:8080/ping) and see detailed logs.

## VIRID research prototype

`ModeVIRID` is a correctness and falsification prototype for the algebraic
router described in the accompanying research. It is deliberately not
presented as a production-speed router.

For each normalized route variant, it constructs an ideal

```text
Jr = <z - routeCode, constraint1, constraint2, ...>
```

where a constraint evaluates to zero when satisfied and to a finite-field
unit when it fails. A compacted generation enumerates the exact generators of
the product ideal `J1 * J2 * ... * Jn`. After substituting a request, the GCD
of the resulting univariate polynomials has precisely the matching route codes
as roots.

The implementation also contains the mechanisms needed to test the complete
proposal:

- immutable snapshots published through `atomic.Pointer`
- one immediate delta generation per inserted route
- logical delete tombstones
- explicit generation compaction
- exact priority resolution and final route verification
- parameter, one-segment wildcard, suffix catch-all, and finite optional segments
- host, HTTP method, API version, and explicit priority constraints
- bounded compilation with an exact linear-oracle fallback
- structured lookup traces exposing basis size, non-zero specializations, GCD coefficients, and decoded roots

### Pattern adapter used by the prototype

| Syntax | Meaning |
|---|---|
| `/users` | static segment |
| `/users/:id` | capturing one-segment parameter |
| `/health/?` | non-capturing one-segment wildcard |
| `/assets/*path` | capturing suffix catch-all, including an empty suffix |
| `/blog/:year/:month?` | finite optional segment expansion |

### Engine integration

```go
e := middleware.New()
if err := e.SwitchRouter(middleware.ModeVIRID); err != nil {
    log.Fatal(err)
}
```

### Inspecting the algebra

The debug command builds a representative route set, optionally compacts its
delta generations, performs a lookup, and prints a JSON proof trace.

```bash
go run ./cmd/virid-debug -path /users/42 -compact=true
```

Useful variants:

```bash
# Exercise host and version constraints.
go run ./cmd/virid-debug \
  -path /api/42 \
  -host api.example.com \
  -version v2

# Force compilation to exceed its cap and verify exact fallback behavior.
go run ./cmd/virid-debug \
  -path /users/42 \
  -max-basis 100
```

The default debug corpus has seven logical routes and eight optional-expanded
variants. At the time of writing it produces 92,160 factored product-ideal
generators after compaction. This is intentional: the tool exposes rather than
hides the generator-growth problem.

### Verification

```bash
go test ./...
go test -race ./...
go test ./middleware -run VIRID -count=1
go test ./middleware -bench VIRID -benchmem
```

The tests include finite-field polynomial arithmetic, the unit-collapse
property, multiple algebraic roots, priority conflicts, parameters, optional
segments, host/version constraints, fallback, tombstones, concurrent snapshot
readers, and randomized differential comparison with an independent linear
oracle.

The initial correctness and microbenchmark record is in
[`docs/virid-validation.md`](docs/virid-validation.md).

### Important limitations

- The prototype enumerates a complete product-ideal generating set; it does
  not yet implement Gröbner/F4/F5 compression.
- Constraint atoms use exact Go comparisons and expose their result as a
  zero/unit field value. A production implementation would need a separately
  measured exact symbolization frontend.
- Root recovery tests the resulting polynomial against the generation's known
  route-code domain. General finite-field factorization is not implemented.
- Compiled lookup can be much slower and larger than Trie/Radix lookup. The
  code exists to determine when the proposal fails as well as when it works.
- Reaching the generator cap switches that generation to a complete linear
  oracle. It never silently drops a route.

## License

MIT License. See [LICENSE](LICENSE) for details.
