package middleware

import (
	"testing"
)

func TestAhoCorasickRouter_Basic(t *testing.T) {
	router := NewAhoCorasickRouter()

	router.AddRoute(methodGET, "/", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/ping", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/users/:id", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/posts/:post_id/comments/:comment_id", []HandlerFunc{func(c *Context) {}})

	tests := []struct {
		path           string
		expected       bool
		expectedParams map[string]string
	}{
		{"/", true, nil},
		{"/ping", true, nil},
		{"/users/123", true, map[string]string{"id": "123"}},
		{"/users/abc", true, map[string]string{"id": "abc"}},
		{"/posts/1/comments/2", true, map[string]string{"post_id": "1", "comment_id": "2"}},
		{"/notfound", false, nil},
		{"/users", false, nil},
	}

	for _, tt := range tests {
		handlers, params, found := router.Find(methodGET, tt.path)
		if found != tt.expected {
			t.Errorf("path %s: expected found=%v, got %v", tt.path, tt.expected, found)
		}
		if tt.expected && tt.expectedParams != nil {
			for k, v := range tt.expectedParams {
				if params[k] != v {
					t.Errorf("path %s: expected params[%s]=%s, got %s", tt.path, k, v, params[k])
				}
			}
		}
		if tt.expected && len(handlers) == 0 {
			t.Errorf("path %s: expected handlers, got none", tt.path)
		}
	}
}

func TestAhoCorasickRouter_Methods(t *testing.T) {
	router := NewAhoCorasickRouter()

	router.AddRoute(methodGET, "/resource", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodPOST, "/resource", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodPUT, "/resource", []HandlerFunc{func(c *Context) {}})

	_, _, foundGET := router.Find(methodGET, "/resource")
	_, _, foundPOST := router.Find(methodPOST, "/resource")
	_, _, foundPUT := router.Find(methodPUT, "/resource")
	_, _, foundDELETE := router.Find(methodDELETE, "/resource")

	if !foundGET {
		t.Error("expected GET to be found")
	}
	if !foundPOST {
		t.Error("expected POST to be found")
	}
	if !foundPUT {
		t.Error("expected PUT to be found")
	}
	if foundDELETE {
		t.Error("expected DELETE to not be found")
	}
}

func TestAhoCorasickRouter_Groups(t *testing.T) {
	router := NewAhoCorasickRouter()

	router.AddRoute(methodGET, "/api/v1/users", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/api/v1/posts", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/api/v2/users", []HandlerFunc{func(c *Context) {}})

	tests := []struct {
		path     string
		expected bool
	}{
		{"/api/v1/users", true},
		{"/api/v1/posts", true},
		{"/api/v2/users", true},
		{"/api/v1/comments", false},
		{"/api/v3/users", false},
	}

	for _, tt := range tests {
		_, _, found := router.Find(methodGET, tt.path)
		if found != tt.expected {
			t.Errorf("path %s: expected found=%v, got %v", tt.path, tt.expected, found)
		}
	}
}

func TestAhoCorasickRouter_ComplexParams(t *testing.T) {
	router := NewAhoCorasickRouter()

	router.AddRoute(methodGET, "/org/:org_id/team/:team_id", []HandlerFunc{func(c *Context) {}})
	router.AddRoute(methodGET, "/org/:org_id/team/:team_id/members/:member_id", []HandlerFunc{func(c *Context) {}})

	_, params, found := router.Find(methodGET, "/org/42/team/100/members/999")
	if !found {
		t.Error("expected to find route")
	}
	if params["org_id"] != "42" {
		t.Errorf("expected org_id=42, got %s", params["org_id"])
	}
	if params["team_id"] != "100" {
		t.Errorf("expected team_id=100, got %s", params["team_id"])
	}
	if params["member_id"] != "999" {
		t.Errorf("expected member_id=999, got %s", params["member_id"])
	}
}

func TestAhoCorasickRouter_Middleware(t *testing.T) {
	router := NewAhoCorasickRouter()

	router.Use(func(c *Context) {})
	router.Use(func(c *Context) {})

	router.AddRoute(methodGET, "/test", []HandlerFunc{func(c *Context) {}})

	handlers, _, found := router.Find(methodGET, "/test")
	if !found {
		t.Error("expected to find route")
	}
	if len(handlers) != 3 {
		t.Errorf("expected 3 handlers (2 middleware + 1 route), got %d", len(handlers))
	}
}

func TestAhoCorasickRouter_Engine(t *testing.T) {
	e := New()
	err := e.SwitchRouter(ModeAhoCorasick)
	if err != nil {
		t.Fatalf("failed to switch router: %v", err)
	}

	e.GET("/test/:id", func(c *Context) {})

	handlers, params, found := e.router.Find(methodGET, "/test/123")
	if !found {
		t.Error("expected to find route")
	}
	if len(handlers) == 0 {
		t.Error("expected handlers")
	}
	if params["id"] != "123" {
		t.Errorf("expected id=123, got %s", params["id"])
	}
}

func BenchmarkAhoCorasickRouter_Simple(b *testing.B) {
	router := NewAhoCorasickRouter()
	router.AddRoute(methodGET, "/ping", []HandlerFunc{func(c *Context) {}})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/ping")
	}
}

func BenchmarkAhoCorasickRouter_Params(b *testing.B) {
	router := NewAhoCorasickRouter()
	router.AddRoute(methodGET, "/users/:id/posts/:post_id", []HandlerFunc{func(c *Context) {}})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/users/123/posts/456")
	}
}

func BenchmarkAhoCorasickRouter_Deep(b *testing.B) {
	router := NewAhoCorasickRouter()
	router.AddRoute(methodGET, "/a/b/c/d/e/f/g/h/i/j", []HandlerFunc{func(c *Context) {}})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/a/b/c/d/e/f/g/h/i/j")
	}
}
