package benchmarklab

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v3"
	"github.com/julienschmidt/httprouter"
	"github.com/labstack/echo/v4"
	project "middleware-go/middleware"
)

type discardWriter struct {
	header http.Header
	status int
}

func newDiscardWriter() *discardWriter               { return &discardWriter{header: make(http.Header)} }
func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardWriter) WriteHeader(status int)      { w.status = status }

type nativeFactory struct {
	name  string
	build func([]routeSpec) http.Handler
}

func nativeFactories() []nativeFactory {
	return []nativeFactory{
		{name: "net-http-servemux", build: buildServeMux},
		{name: "project-trie", build: buildProjectEngine},
		{name: "httprouter", build: buildHTTPRouter},
		{name: "chi", build: buildChi},
		{name: "gin", build: buildGin},
		{name: "echo", build: buildEcho},
	}
}

func buildServeMux(routes []routeSpec) http.Handler {
	r := http.NewServeMux()
	for _, route := range routes {
		r.HandleFunc(http.MethodGet+" "+route.pattern(syntaxBraces), func(http.ResponseWriter, *http.Request) {})
	}
	return r
}

func buildProjectEngine(routes []routeSpec) http.Handler {
	e := project.New()
	for _, route := range routes {
		if err := e.GET(route.pattern(syntaxProject), func(*project.Context) {}); err != nil {
			panic(err)
		}
	}
	return e
}

func buildHTTPRouter(routes []routeSpec) http.Handler {
	r := httprouter.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	for _, route := range routes {
		r.GET(route.pattern(syntaxColon), func(http.ResponseWriter, *http.Request, httprouter.Params) {})
	}
	return r
}

func buildChi(routes []routeSpec) http.Handler {
	r := chi.NewRouter()
	for _, route := range routes {
		r.Get(route.pattern(syntaxChi), func(http.ResponseWriter, *http.Request) {})
	}
	return r
}

func buildGin(routes []routeSpec) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	for _, route := range routes {
		r.GET(route.pattern(syntaxColon), func(*gin.Context) {})
	}
	return r
}

func buildEcho(routes []routeSpec) http.Handler {
	e := echo.New()
	for _, route := range routes {
		e.GET(route.pattern(syntaxStar), func(echo.Context) error { return nil })
	}
	return e
}

func BenchmarkNativeHTTPDispatch(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_NATIVE_SIZES", []int{100, 1_000})
	for _, size := range sizes {
		routes := makeCorpus(size)
		for _, factory := range nativeFactories() {
			b.Run(factory.name+"/routes="+itoa(size), func(b *testing.B) {
				handler := factory.build(routes)
				for _, workload := range []string{"hit", "mixed", "miss"} {
					paths := benchmarkPaths(routes, workload)
					requests := makeRequests(paths)
					b.Run(workload, func(b *testing.B) {
						w := newDiscardWriter()
						handler.ServeHTTP(w, requests[0])
						b.ReportAllocs()
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							handler.ServeHTTP(w, requests[i%len(requests)])
						}
					})
				}
			})
		}
	}
}

func BenchmarkNativeHTTPDispatchParallel(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_PARALLEL_SIZES", []int{1_000})
	for _, size := range sizes {
		routes := makeCorpus(size)
		paths := benchmarkPaths(routes, "mixed")
		for _, factory := range nativeFactories() {
			b.Run(factory.name+"/routes="+itoa(size), func(b *testing.B) {
				handler := factory.build(routes)
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					w := newDiscardWriter()
					i := 0
					for pb.Next() {
						req := httptest.NewRequest(http.MethodGet, paths[i%len(paths)], nil)
						handler.ServeHTTP(w, req)
						i++
					}
				})
			})
		}
	}
}

func BenchmarkFiberProtocolBridge(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_FIBER_SIZES", []int{100})
	for _, size := range sizes {
		routes := makeCorpus(size)
		app := fiber.New()
		for _, route := range routes {
			app.Get(route.pattern(syntaxStar), func(fiber.Ctx) error { return nil })
		}
		requests := makeRequests(benchmarkPaths(routes, "mixed"))
		b.Run("fiber/app.Test/routes="+itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				response, err := app.Test(requests[i%len(requests)], fiber.TestConfig{Timeout: 0})
				if err != nil {
					b.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
			}
		})
	}
}

func makeRequests(paths []string) []*http.Request {
	requests := make([]*http.Request, len(paths))
	for i, path := range paths {
		requests[i] = httptest.NewRequest(http.MethodGet, path, nil)
	}
	return requests
}

func itoa(value int) string { return strconv.Itoa(value) }
