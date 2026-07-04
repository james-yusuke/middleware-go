# middleware-go

A simple and extensible HTTP middleware engine for Go, inspired by the elegant Gin style syntax.

## Features

- Minimal core, easy to read and extend
- Gin-like handler registration and middleware style
- Detailed logging and debug mode support
- Custom handler and middleware chaining
- No third-party dependencies

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

## License

MIT License. See [LICENSE](LICENSE) for details.
