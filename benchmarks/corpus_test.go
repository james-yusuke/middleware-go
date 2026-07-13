package benchmarklab

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

type routeKind uint8

const (
	staticRoute routeKind = iota
	parameterRoute
	catchAllRoute
	multiParameterRoute
)

type routeSpec struct {
	id      int
	kind    routeKind
	hitPath string
}

func makeCorpus(n int) []routeSpec {
	routes := make([]routeSpec, n)
	for i := range routes {
		prefix := fmt.Sprintf("/r/r%06d", i)
		routes[i] = routeSpec{id: i, kind: routeKind(i % 4)}
		switch routes[i].kind {
		case staticRoute:
			routes[i].hitPath = fmt.Sprintf("%s/static/s%06d", prefix, i)
		case parameterRoute:
			routes[i].hitPath = prefix + "/users/u42"
		case catchAllRoute:
			routes[i].hitPath = prefix + "/assets/css/site.css"
		case multiParameterRoute:
			routes[i].hitPath = prefix + "/posts/2026/router-design"
		}
	}
	return routes
}

type routeSyntax uint8

const (
	syntaxProject routeSyntax = iota // middleware-go uses an unnamed catch-all
	syntaxColon                      // httprouter and Gin
	syntaxBraces                     // net/http ServeMux
	syntaxChi
	syntaxStar // Echo and Fiber
)

func (r routeSpec) pattern(syntax routeSyntax) string {
	prefix := fmt.Sprintf("/r/r%06d", r.id)
	switch r.kind {
	case staticRoute:
		return fmt.Sprintf("%s/static/s%06d", prefix, r.id)
	case parameterRoute:
		switch syntax {
		case syntaxBraces, syntaxChi:
			return prefix + "/users/{id}"
		default:
			return prefix + "/users/:id"
		}
	case catchAllRoute:
		switch syntax {
		case syntaxProject:
			return prefix + "/assets/*"
		case syntaxBraces:
			return prefix + "/assets/{rest...}"
		case syntaxChi, syntaxStar:
			return prefix + "/assets/*"
		default:
			return prefix + "/assets/*rest"
		}
	case multiParameterRoute:
		switch syntax {
		case syntaxBraces, syntaxChi:
			return prefix + "/posts/{year}/{slug}"
		default:
			return prefix + "/posts/:year/:slug"
		}
	default:
		panic("unknown route kind")
	}
}

func benchmarkSizes(b *testing.B, env string, defaults []int) []int {
	b.Helper()
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return defaults
	}
	parts := strings.Split(raw, ",")
	sizes := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n <= 0 {
			b.Fatalf("%s contains invalid route count %q", env, part)
		}
		sizes = append(sizes, n)
	}
	return sizes
}

func benchmarkPaths(routes []routeSpec, workload string) []string {
	paths := make([]string, len(routes))
	for i, route := range routes {
		switch workload {
		case "hit":
			paths[i] = route.hitPath
		case "miss":
			paths[i] = fmt.Sprintf("/missing/r%06d/no-route", i)
		case "mixed":
			if i%5 == 0 {
				paths[i] = fmt.Sprintf("/missing/r%06d/no-route", i)
			} else {
				paths[i] = route.hitPath
			}
		default:
			panic("unknown workload")
		}
	}
	return paths
}
