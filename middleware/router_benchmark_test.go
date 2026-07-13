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
		{name: "virid", factory: func() Router { return NewVIRIDRouter() }},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			r := bm.factory()
			_ = r.AddRoute(methodGET, "/", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/ping", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/users/:id/posts/:post_id", []HandlerFunc{func(c *Context) {}})
			_ = r.AddRoute(methodGET, "/assets/*", []HandlerFunc{func(c *Context) {}})
			if virid, ok := r.(*VIRIDRouter); ok {
				_ = virid.Compact()
			}

			paths := []string{"/", "/ping", "/users/42/posts/100", "/assets/css/site.css"}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _ = r.Find(methodGET, paths[i%len(paths)])
			}
		})
	}
}

func BenchmarkVIRIDResearchModes(b *testing.B) {
	build := func(maxBasis int, compact bool) *VIRIDRouter {
		router := NewVIRIDRouterWithOptions(VIRIDOptions{MaxBasisGenerators: maxBasis})
		_ = router.AddRoute(methodGET, "/", []HandlerFunc{func(*Context) {}})
		_ = router.AddRoute(methodGET, "/users", []HandlerFunc{func(*Context) {}})
		_ = router.AddRoute(methodGET, "/users/:id", []HandlerFunc{func(*Context) {}})
		_ = router.AddRoute(methodGET, "/assets/*path", []HandlerFunc{func(*Context) {}})
		if compact {
			_ = router.Compact()
		}
		return router
	}
	paths := []string{"/", "/users", "/users/42", "/assets/css/site.css", "/missing"}

	benchmarks := []struct {
		name   string
		router *VIRIDRouter
	}{
		{name: "delta-generations", router: build(defaultVIRIDMaxBasisGenerators, false)},
		{name: "compiled-product-ideal", router: build(defaultVIRIDMaxBasisGenerators, true)},
		{name: "basis-cap-fallback", router: build(8, true)},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = benchmark.router.Lookup(VIRIDRequest{
					MethodBit: methodGET,
					Path:      paths[i%len(paths)],
				})
			}
		})
	}
}
