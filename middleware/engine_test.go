package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEngineServeHTTPWithParamsAndJSON(t *testing.T) {
	e := New()
	if err := e.GET("/users/:id", func(c *Context) {
		c.JSON(http.StatusOK, map[string]string{"id": c.Param("id")})
	}); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["id"] != "42" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestEngineGlobalMiddlewareRunsOnceInEveryRouterMode(t *testing.T) {
	modes := []RouterMode{ModeTrie, ModeRegex, ModeAhoCorasick, ModeVIRID}
	for _, mode := range modes {
		t.Run(routerModeName(mode), func(t *testing.T) {
			e := New()
			if err := e.SwitchRouter(mode); err != nil {
				t.Fatalf("SwitchRouter failed: %v", err)
			}
			count := 0
			e.Use(func(c *Context) {
				count++
				c.Next()
			})
			if err := e.GET("/ping", func(c *Context) { c.String(http.StatusOK, "pong") }); err != nil {
				t.Fatalf("GET failed: %v", err)
			}

			rr := httptest.NewRecorder()
			e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ping", nil))

			if rr.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rr.Code)
			}
			if count != 1 {
				t.Fatalf("global middleware should run exactly once, ran %d times", count)
			}
		})
	}
}

func TestEngineGroupMiddlewareOrder(t *testing.T) {
	e := New()
	seen := []string{}
	e.Use(func(c *Context) {
		seen = append(seen, "global:before")
		c.Next()
		seen = append(seen, "global:after")
	})
	api := e.Group("/api", func(c *Context) {
		seen = append(seen, "group:before")
		c.Next()
		seen = append(seen, "group:after")
	})
	if err := api.GET("/items", func(c *Context) {
		seen = append(seen, "handler")
		c.String(http.StatusOK, "ok")
	}); err != nil {
		t.Fatalf("group GET failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/items", nil))

	want := []string{"global:before", "group:before", "handler", "group:after", "global:after"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("unexpected order:\n got: %#v\nwant: %#v", seen, want)
	}
}

func TestEngineNotFoundAndUnsupportedMethod(t *testing.T) {
	e := New()
	if err := e.GET("/exists", func(c *Context) { c.String(http.StatusOK, "ok") }); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	notFound := httptest.NewRecorder()
	e.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing route, got %d", notFound.Code)
	}

	unsupported := httptest.NewRecorder()
	e.ServeHTTP(unsupported, httptest.NewRequest(http.MethodTrace, "/exists", nil))
	if unsupported.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for unsupported method, got %d", unsupported.Code)
	}
}

func TestEngineHandleRejectsUnsupportedMethod(t *testing.T) {
	e := New()
	if err := e.Handle("BREW", "/coffee", func(c *Context) {}); err == nil {
		t.Fatal("expected unsupported method error")
	}
}

func TestEngineSwitchRouterRejectsUnknownMode(t *testing.T) {
	e := New()
	err := e.SwitchRouter(RouterMode(999))
	if err == nil {
		t.Fatal("expected error for unknown router mode")
	}
	if e.mode != ModeTrie {
		t.Fatalf("router mode should remain ModeTrie, got %v", e.mode)
	}
}

func TestRecoverMiddlewareWritesInternalServerError(t *testing.T) {
	e := New()
	e.Use(Recover())
	if err := e.GET("/boom", func(c *Context) { panic("boom") }); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestLimitBodyMiddleware(t *testing.T) {
	e := New()
	e.Use(LimitBody(3))
	var readErr error
	if err := e.POST("/echo", func(c *Context) {
		_, readErr = io.ReadAll(c.R.Body)
		if readErr != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, "ok")
	}); err != nil {
		t.Fatalf("POST failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/echo", stringsReader("1234")))

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
	if readErr == nil {
		t.Fatal("expected body reader to return an error")
	}
}

func TestTimeoutMiddlewareInjectsDeadline(t *testing.T) {
	e := New()
	e.Use(Timeout(time.Second))
	var hasDeadline bool
	if err := e.GET("/deadline", func(c *Context) {
		_, hasDeadline = c.R.Context().Deadline()
		c.String(http.StatusOK, "ok")
	}); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/deadline", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !hasDeadline {
		t.Fatal("expected request context to have a deadline")
	}
}

func TestContextPoolResetsPerRequestState(t *testing.T) {
	e := New()
	requests := 0
	if err := e.GET("/keys", func(c *Context) {
		requests++
		if _, ok := c.Get("request-scoped"); ok {
			t.Fatal("request-scoped key leaked from a previous request")
		}
		c.Set("request-scoped", requests)
		c.String(http.StatusOK, "ok")
	}); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/keys", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, rr.Code)
		}
	}
}

func TestRecoverMiddlewareDoesNotHideNormalErrors(t *testing.T) {
	e := New()
	e.Use(Recover())
	wantErr := errors.New("ordinary error")
	var gotErr error
	if err := e.GET("/ordinary", func(c *Context) {
		gotErr = wantErr
		c.String(http.StatusAccepted, "accepted")
	}); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ordinary", nil))

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("ordinary error was not preserved: %v", gotErr)
	}
}

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func routerModeName(mode RouterMode) string {
	switch mode {
	case ModeTrie:
		return "trie"
	case ModeRegex:
		return "regex"
	case ModeAhoCorasick:
		return "aho-corasick"
	case ModeVIRID:
		return "virid"
	default:
		return "unknown"
	}
}
