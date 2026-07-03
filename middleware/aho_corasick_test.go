package middleware

import "testing"

func TestAhoCorasickRouterBasic(t *testing.T) {
	router := NewAhoCorasickRouter()
	mustAddRoute(t, router, methodGET, "/", mark("root"))
	mustAddRoute(t, router, methodGET, "/ping", mark("ping"))
	mustAddRoute(t, router, methodGET, "/users/:id", mark("user"))
	mustAddRoute(t, router, methodGET, "/posts/:post_id/comments/:comment_id", mark("comment"))
	mustAddRoute(t, router, methodGET, "/assets/*", mark("asset"))

	tests := []struct {
		path           string
		expected       bool
		expectedSeen   []string
		expectedParams map[string]string
	}{
		{path: "/", expected: true, expectedSeen: []string{"root"}},
		{path: "/ping", expected: true, expectedSeen: []string{"ping"}},
		{path: "/users/123", expected: true, expectedSeen: []string{"user"}, expectedParams: map[string]string{"id": "123"}},
		{path: "/users/abc", expected: true, expectedSeen: []string{"user"}, expectedParams: map[string]string{"id": "abc"}},
		{path: "/posts/1/comments/2", expected: true, expectedSeen: []string{"comment"}, expectedParams: map[string]string{"post_id": "1", "comment_id": "2"}},
		{path: "/assets/images/logo.png", expected: true, expectedSeen: []string{"asset"}, expectedParams: map[string]string{"*": "images/logo.png"}},
		{path: "/notfound", expected: false},
		{path: "/users", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			handlers, params, found := router.Find(methodGET, tt.path)
			if found != tt.expected {
				t.Fatalf("expected found=%v, got %v", tt.expected, found)
			}
			if !tt.expected {
				return
			}
			assertSeen(t, runHandlerChain(handlers), tt.expectedSeen)
			for k, v := range tt.expectedParams {
				if params[k] != v {
					t.Fatalf("expected params[%s]=%s, got %s", k, v, params[k])
				}
			}
		})
	}
}

func TestAhoCorasickRouterMethods(t *testing.T) {
	router := NewAhoCorasickRouter()
	mustAddRoute(t, router, methodGET, "/resource", mark("get"))
	mustAddRoute(t, router, methodPOST, "/resource", mark("post"))
	mustAddRoute(t, router, methodPUT, "/resource", mark("put"))

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

func TestAhoCorasickRouterGroups(t *testing.T) {
	router := NewAhoCorasickRouter()
	mustAddRoute(t, router, methodGET, "/api/v1/users", mark("v1-users"))
	mustAddRoute(t, router, methodGET, "/api/v1/posts", mark("v1-posts"))
	mustAddRoute(t, router, methodGET, "/api/v2/users", mark("v2-users"))

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

func TestAhoCorasickRouterComplexParams(t *testing.T) {
	router := NewAhoCorasickRouter()
	mustAddRoute(t, router, methodGET, "/org/:org_id/team/:team_id", mark("team"))
	mustAddRoute(t, router, methodGET, "/org/:org_id/team/:team_id/members/:member_id", mark("member"))

	_, params, found := router.Find(methodGET, "/org/42/team/100/members/999")
	if !found {
		t.Fatal("expected to find route")
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

func TestAhoCorasickRouterMiddleware(t *testing.T) {
	router := NewAhoCorasickRouter()
	router.Use(mark("mw1"))
	router.Use(mark("mw2"))
	mustAddRoute(t, router, methodGET, "/test", mark("handler"))

	handlers, _, found := router.Find(methodGET, "/test")
	if !found {
		t.Fatal("expected to find route")
	}
	assertSeen(t, runHandlerChain(handlers), []string{"mw1", "mw2", "handler"})
}

func TestAhoCorasickRouterEngine(t *testing.T) {
	e := New()
	err := e.SwitchRouter(ModeAhoCorasick)
	if err != nil {
		t.Fatalf("failed to switch router: %v", err)
	}
	if err := e.GET("/test/:id", func(c *Context) {}); err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	handlers, params, found := e.router.Find(methodGET, "/test/123")
	if !found {
		t.Fatal("expected to find route")
	}
	if len(handlers) == 0 {
		t.Fatal("expected handlers")
	}
	if params["id"] != "123" {
		t.Errorf("expected id=123, got %s", params["id"])
	}
}

func BenchmarkAhoCorasickRouterSimple(b *testing.B) {
	router := NewAhoCorasickRouter()
	_ = router.AddRoute(methodGET, "/ping", []HandlerFunc{func(c *Context) {}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/ping")
	}
}

func BenchmarkAhoCorasickRouterParams(b *testing.B) {
	router := NewAhoCorasickRouter()
	_ = router.AddRoute(methodGET, "/users/:id/posts/:post_id", []HandlerFunc{func(c *Context) {}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/users/123/posts/456")
	}
}

func BenchmarkAhoCorasickRouterDeep(b *testing.B) {
	router := NewAhoCorasickRouter()
	_ = router.AddRoute(methodGET, "/a/b/c/d/e/f/g/h/i/j", []HandlerFunc{func(c *Context) {}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Find(methodGET, "/a/b/c/d/e/f/g/h/i/j")
	}
}
