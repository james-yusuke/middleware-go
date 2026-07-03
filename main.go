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
