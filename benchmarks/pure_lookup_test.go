package benchmarklab

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/julienschmidt/httprouter"
	"github.com/labstack/echo/v4"
	project "middleware-go/middleware"
)

type pureLookup interface {
	name() string
	add(routeSpec) error
	lookup(string) bool
}

type projectLookup struct {
	label  string
	router project.Router
	method int
}

func newProjectLookup(label string, router project.Router) pureLookup {
	method, err := project.HTTPMethodBit(http.MethodGet)
	if err != nil {
		panic(err)
	}
	return &projectLookup{label: label, router: router, method: method}
}

func (r *projectLookup) name() string { return r.label }
func (r *projectLookup) add(spec routeSpec) error {
	return r.router.AddRoute(r.method, spec.pattern(syntaxProject), []project.HandlerFunc{func(*project.Context) {}})
}
func (r *projectLookup) lookup(path string) bool {
	_, _, found := r.router.Find(r.method, path)
	return found
}

type httpRouterLookup struct{ router *httprouter.Router }

func newHTTPRouterLookup() pureLookup    { return &httpRouterLookup{router: httprouter.New()} }
func (r *httpRouterLookup) name() string { return "httprouter" }
func (r *httpRouterLookup) add(spec routeSpec) error {
	r.router.GET(spec.pattern(syntaxColon), func(http.ResponseWriter, *http.Request, httprouter.Params) {})
	return nil
}
func (r *httpRouterLookup) lookup(path string) bool {
	handler, _, _ := r.router.Lookup(http.MethodGet, path)
	return handler != nil
}

type chiLookup struct {
	router *chi.Mux
	pool   sync.Pool
}

func newChiLookup() pureLookup {
	r := &chiLookup{router: chi.NewRouter()}
	r.pool.New = func() any { return chi.NewRouteContext() }
	return r
}
func (r *chiLookup) name() string { return "chi" }
func (r *chiLookup) add(spec routeSpec) error {
	r.router.Get(spec.pattern(syntaxChi), func(http.ResponseWriter, *http.Request) {})
	return nil
}
func (r *chiLookup) lookup(path string) bool {
	ctx := r.pool.Get().(*chi.Context)
	ctx.Reset()
	found := r.router.Match(ctx, http.MethodGet, path)
	r.pool.Put(ctx)
	return found
}

type echoLookup struct {
	echo   *echo.Echo
	req    *http.Request
	writer *discardWriter
}

func newEchoLookup() pureLookup {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := newDiscardWriter()
	return &echoLookup{echo: e, req: req, writer: w}
}
func (r *echoLookup) name() string { return "echo" }
func (r *echoLookup) add(spec routeSpec) error {
	r.echo.GET(spec.pattern(syntaxStar), func(echo.Context) error { return nil })
	return nil
}
func (r *echoLookup) lookup(path string) bool {
	ctx := r.echo.AcquireContext()
	ctx.Reset(r.req, r.writer)
	r.echo.Router().Find(http.MethodGet, path, ctx)
	found := ctx.Path() != ""
	r.echo.ReleaseContext(ctx)
	return found
}

type pureFactory struct {
	name string
	new  func() pureLookup
}

func pureFactories() []pureFactory {
	return []pureFactory{
		{name: "project-trie", new: func() pureLookup { return newProjectLookup("project-trie", project.NewTrieRouter()) }},
		{name: "project-regex", new: func() pureLookup { return newProjectLookup("project-regex", project.NewRegexRouter()) }},
		{name: "project-aho-corasick", new: func() pureLookup { return newProjectLookup("project-aho-corasick", project.NewAhoCorasickRouter()) }},
		{name: "httprouter", new: newHTTPRouterLookup},
		{name: "chi", new: newChiLookup},
		{name: "echo", new: newEchoLookup},
	}
}

func selectedPureFactories(b *testing.B) []pureFactory {
	b.Helper()
	factories := pureFactories()
	raw := strings.TrimSpace(os.Getenv("ROUTER_BENCH_ROUTERS"))
	if raw == "" {
		return factories
	}
	wanted := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		wanted[strings.TrimSpace(name)] = true
	}
	selected := make([]pureFactory, 0, len(wanted))
	for _, factory := range factories {
		if wanted[factory.name] {
			selected = append(selected, factory)
			delete(wanted, factory.name)
		}
	}
	if len(wanted) != 0 {
		for name := range wanted {
			b.Fatalf("ROUTER_BENCH_ROUTERS contains unknown router %q", name)
		}
	}
	return selected
}

func BenchmarkPureLookup(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_BENCH_SIZES", []int{100, 1_000, 10_000})
	for _, size := range sizes {
		routes := makeCorpus(size)
		for _, factory := range selectedPureFactories(b) {
			b.Run(factory.name+"/routes="+itoa(size), func(b *testing.B) {
				router := factory.new()
				for _, route := range routes {
					if err := router.add(route); err != nil {
						b.Fatal(err)
					}
				}
				for _, workload := range []string{"hit", "mixed", "miss"} {
					paths := benchmarkPaths(routes, workload)
					b.Run(workload, func(b *testing.B) {
						benchmarkPureLookup(b, router, paths)
					})
				}
			})
		}
	}
}

func BenchmarkPureLookupParallel(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_PARALLEL_SIZES", []int{1_000})
	for _, size := range sizes {
		routes := makeCorpus(size)
		paths := benchmarkPaths(routes, "mixed")
		for _, factory := range selectedPureFactories(b) {
			b.Run(factory.name+"/routes="+itoa(size), func(b *testing.B) {
				router := factory.new()
				for _, route := range routes {
					if err := router.add(route); err != nil {
						b.Fatal(err)
					}
				}
				// Some implementations finalize an immutable lookup structure on
				// first use. Keep that one-time build out of the concurrent region.
				_ = router.lookup(paths[0])
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					i := 0
					for pb.Next() {
						_ = router.lookup(paths[i%len(paths)])
						i++
					}
				})
			})
		}
	}
}

func benchmarkPureLookup(b *testing.B, router pureLookup, paths []string) {
	b.Helper()
	for _, path := range paths {
		_ = router.lookup(path)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = router.lookup(paths[i%len(paths)])
	}
}
