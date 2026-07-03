package middleware

import (
	"reflect"
	"testing"
)

func mark(name string) HandlerFunc {
	return func(c *Context) {
		seen, _ := c.Get("seen")
		if seen == nil {
			return
		}
		list := seen.(*[]string)
		*list = append(*list, name)
	}
}

func markAround(name string) HandlerFunc {
	return func(c *Context) {
		seen, _ := c.Get("seen")
		list := seen.(*[]string)
		*list = append(*list, name+":before")
		c.Next()
		*list = append(*list, name+":after")
	}
}

func runHandlerChain(handlers []HandlerFunc) []string {
	seen := []string{}
	c := &Context{
		Keys:     map[string]interface{}{"seen": &seen},
		handlers: handlers,
		index:    -1,
	}
	c.Next()
	return seen
}

func assertSeen(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("handler order mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}
