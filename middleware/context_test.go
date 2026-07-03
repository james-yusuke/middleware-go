package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestContextParamSetGet(t *testing.T) {
	c := &Context{}
	if got := c.Param("id"); got != "" {
		t.Fatalf("nil params should return empty string, got %q", got)
	}

	c.Params = map[string]string{"id": "42"}
	if got := c.Param("id"); got != "42" {
		t.Fatalf("expected param id=42, got %q", got)
	}

	if _, ok := c.Get("missing"); ok {
		t.Fatal("missing key should not exist")
	}
	c.Set("role", "admin")
	got, ok := c.Get("role")
	if !ok || got != "admin" {
		t.Fatalf("expected role=admin, got value=%v ok=%v", got, ok)
	}
}

func TestContextNextAndAbort(t *testing.T) {
	seen := []string{}
	c := &Context{index: -1}
	c.handlers = []HandlerFunc{
		func(c *Context) { seen = append(seen, "first") },
		func(c *Context) {
			seen = append(seen, "second")
			c.Abort()
		},
		func(c *Context) { seen = append(seen, "third") },
	}

	c.Next()

	want := []string{"first", "second"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("unexpected handler order: got %#v want %#v", seen, want)
	}
}

func TestContextJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	c := &Context{W: rr}

	c.JSON(http.StatusCreated, map[string]string{"id": "42"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["id"] != "42" {
		t.Fatalf("unexpected JSON body: %#v", body)
	}
}

func TestContextString(t *testing.T) {
	rr := httptest.NewRecorder()
	c := &Context{W: rr}

	c.String(http.StatusAccepted, "hello %s", "world")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := rr.Body.String(); got != "hello world" {
		t.Fatalf("unexpected body: %q", got)
	}
}
