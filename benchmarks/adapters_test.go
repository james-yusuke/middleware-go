package benchmarklab

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPureLookupAdapters(t *testing.T) {
	routes := makeCorpus(32)
	for _, factory := range pureFactories() {
		t.Run(factory.name, func(t *testing.T) {
			router := factory.new()
			for _, route := range routes {
				if err := router.add(route); err != nil {
					t.Fatal(err)
				}
			}
			for _, route := range routes {
				if !router.lookup(route.hitPath) {
					t.Fatalf("expected hit for %q", route.hitPath)
				}
			}
			if router.lookup("/definitely/missing") {
				t.Fatal("unexpected match for missing path")
			}
		})
	}
}

func TestNativeHTTPAdapters(t *testing.T) {
	routes := makeCorpus(32)
	for _, factory := range nativeFactories() {
		t.Run(factory.name, func(t *testing.T) {
			handler := factory.build(routes)
			for _, route := range routes {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, route.hitPath, nil))
				if recorder.Code == http.StatusNotFound {
					t.Fatalf("expected hit for %q", route.hitPath)
				}
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/definitely/missing", nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("missing path status = %d, want 404", recorder.Code)
			}
		})
	}
}

func TestFiberAdapter(t *testing.T) {
	routes := makeCorpus(32)
	app := fiber.New()
	for _, route := range routes {
		app.Get(route.pattern(syntaxStar), func(fiber.Ctx) error { return nil })
	}
	for _, path := range []string{routes[0].hitPath, routes[1].hitPath, routes[2].hitPath, routes[3].hitPath} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), fiber.TestConfig{Timeout: 0})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode == http.StatusNotFound {
			t.Fatalf("expected hit for %q", path)
		}
	}
}
