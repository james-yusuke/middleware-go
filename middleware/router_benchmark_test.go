package middleware

import "testing"

func BenchmarkRouterContractFind(b *testing.B) {
	benchmarks := []struct {
		name    string
		factory func() Router
	}{
		{name: "trie", factory: func() Router { return NewTrieRouter() }},
		{name: "regex", factory: func() Router { return NewRegexRouter() }},
		{name: "aho-corasick", factory: func() Router { return NewAhoCorasickRouter() }},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			r := bm.factory()
			_ = r.AddRoute(methodGET, "/", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/ping", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/users/:id/posts/:post_id", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/assets/*", []HandlerFunc{func(c *Context) {}})

			paths := []string{"/", "/ping", "/users/42/posts/100", "/assets/css/site.css"}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _ = r.Find(methodGET, paths[i%len(paths)])
			}
		})
	}
}
