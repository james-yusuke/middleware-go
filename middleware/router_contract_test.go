package middleware

import "testing"

func TestRouterContract(t *testing.T) {
	tests := []struct {
		name    string
		factory func() Router
	}{
		{name: "trie", factory: func() Router { return NewTrieRouter() }},
		{name: "regex", factory: func() Router { return NewRegexRouter() }},
		{name: "aho-corasick", factory: func() Router { return NewAhoCorasickRouter() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.factory()
			r.Use(mark("mw"))
			mustAddRoute(t, r, methodGET, "/", mark("root"))
			mustAddRoute(t, r, methodGET, "/ping", mark("ping"))
			mustAddRoute(t, r, methodGET, "/users/:id", mark("user"))
			mustAddRoute(t, r, methodGET, "/files/:name", mark("file-param"))
			mustAddRoute(t, r, methodGET, "/files/static", mark("file-static"))
			mustAddRoute(t, r, methodGET, "/assets/*", mark("asset-wildcard"))
			mustAddRoute(t, r, methodPOST, "/ping", mark("ping-post"))

			cases := []struct {
				path       string
				methodBit  int
				wantFound  bool
				wantSeen   []string
				wantParams map[string]string
			}{
				{path: "/", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "root"}},
				{path: "/ping", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "ping"}},
				{path: "/ping", methodBit: methodPOST, wantFound: true, wantSeen: []string{"mw", "ping-post"}},
				{path: "/users/42", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "user"}, wantParams: map[string]string{"id": "42"}},
				{path: "/files/static", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "file-static"}},
				{path: "/files/readme.md", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "file-param"}, wantParams: map[string]string{"name": "readme.md"}},
				{path: "/assets/css/site.css", methodBit: methodGET, wantFound: true, wantSeen: []string{"mw", "asset-wildcard"}, wantParams: map[string]string{"*": "css/site.css"}},
				{path: "/missing", methodBit: methodGET, wantFound: false},
				{path: "/ping", methodBit: methodDELETE, wantFound: false},
			}

			for _, tc := range cases {
				t.Run(tc.path, func(t *testing.T) {
					handlers, params, found := r.Find(tc.methodBit, tc.path)
					if found != tc.wantFound {
						t.Fatalf("found=%v, want %v", found, tc.wantFound)
					}
					if !tc.wantFound {
						return
					}
					assertSeen(t, runHandlerChain(handlers), tc.wantSeen)
					for key, want := range tc.wantParams {
						if got := params[key]; got != want {
							t.Fatalf("params[%q]=%q, want %q", key, got, want)
						}
					}
				})
			}
		})
	}
}

func mustAddRoute(t *testing.T, r Router, methodBit int, pattern string, handlers ...HandlerFunc) {
	t.Helper()
	if err := r.AddRoute(methodBit, pattern, handlers); err != nil {
		t.Fatalf("AddRoute(%q) failed: %v", pattern, err)
	}
}
