# middleware-go

A simple and extensible HTTP middleware engine for Go, inspired by the elegant Gin style syntax.

## Features

- Minimal core, easy to read and extend
- Gin-like handler registration and middleware style
- Detailed logging and debug mode support
- Custom handler and middleware chaining
- No third-party dependencies
- Experimental VIRID (Vanishing-Ideal Route Identification and Decoding) router

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
