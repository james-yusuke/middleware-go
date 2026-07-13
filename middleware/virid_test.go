package middleware

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestVIRIDUnitCollapseAndRouteIDRecovery(t *testing.T) {
	router := NewVIRIDRouter()
	usersID := mustAddVIRIDRoute(t, router, methodGET, "/users", VIRIDRouteOptions{})
	userID := mustAddVIRIDRoute(t, router, methodGET, "/users/:id", VIRIDRouteOptions{})
	_ = mustAddVIRIDRoute(t, router, methodGET, "/assets/*", VIRIDRouteOptions{})

	if err := router.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	stats := router.Stats()
	if stats.Generations != 1 || stats.BasisGenerators != 64 {
		t.Fatalf("unexpected compacted stats: %+v", stats)
	}

	request := VIRIDRequest{MethodBit: methodGET, Path: "/users/42"}
	match := router.Lookup(request)
	if !match.Found || match.RouteID != userID {
		t.Fatalf("match=%+v, want route %d", match, userID)
	}
	if got := match.Params["id"]; got != "42" {
		t.Fatalf("id=%q, want 42", got)
	}

	trace := router.Explain(request)
	if trace.FallbackUsed || trace.VerificationFailures != 0 {
		t.Fatalf("unexpected degraded trace: %+v", trace)
	}
	if len(trace.Generations) != 1 {
		t.Fatalf("generations=%d, want 1", len(trace.Generations))
	}
	generation := trace.Generations[0]
	if !generation.CandidateSetAgrees {
		t.Fatalf("algebraic candidates disagree with oracle: %+v", generation)
	}
	if !reflect.DeepEqual(generation.RootCodes, []uint64{2}) {
		t.Fatalf("roots=%v, want [2]", generation.RootCodes)
	}
	if !reflect.DeepEqual(generation.MatchingRouteIDs, []uint64{userID}) {
		t.Fatalf("route ids=%v, want [%d]", generation.MatchingRouteIDs, userID)
	}
	if viridPolyDegree(generation.GCDCoefficients) != 1 {
		t.Fatalf("GCD=%v, want degree 1", generation.GCDCoefficients)
	}

	noMatchTrace := router.Explain(VIRIDRequest{MethodBit: methodGET, Path: "/missing/path"})
	if got := noMatchTrace.Generations[0].GCDCoefficients; !reflect.DeepEqual(got, []uint64{1}) {
		t.Fatalf("no-match GCD=%v, want unit polynomial [1]", got)
	}
	if noMatchTrace.WinnerRouteID != 0 {
		t.Fatalf("no-match winner=%d", noMatchTrace.WinnerRouteID)
	}

	rootMatch := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: "/users"})
	if rootMatch.RouteID != usersID {
		t.Fatalf("/users selected %d, want %d", rootMatch.RouteID, usersID)
	}
}

func TestVIRIDPriorityAndMultipleAlgebraicRoots(t *testing.T) {
	router := NewVIRIDRouter()
	paramID := mustAddVIRIDRoute(t, router, methodGET, "/:id", VIRIDRouteOptions{})
	staticID := mustAddVIRIDRoute(t, router, methodGET, "/users", VIRIDRouteOptions{})
	catchAllID := mustAddVIRIDRoute(t, router, methodGET, "/*rest", VIRIDRouteOptions{})

	if err := router.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	trace := router.Explain(VIRIDRequest{MethodBit: methodGET, Path: "/users"})
	if !reflect.DeepEqual(trace.Generations[0].MatchingRouteIDs, []uint64{paramID, staticID, catchAllID}) {
		t.Fatalf("matching routes=%v", trace.Generations[0].MatchingRouteIDs)
	}
	if degree := viridPolyDegree(trace.Generations[0].GCDCoefficients); degree != 3 {
		t.Fatalf("GCD degree=%d, want 3", degree)
	}
	if trace.WinnerRouteID != staticID {
		t.Fatalf("winner=%d, want static route %d", trace.WinnerRouteID, staticID)
	}

	highPriorityID := mustAddVIRIDRoute(t, router, methodGET, "/*override", VIRIDRouteOptions{
		ExplicitPriority: 100,
	})
	match := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: "/users"})
	if match.RouteID != highPriorityID {
		t.Fatalf("explicit-priority winner=%d, want %d", match.RouteID, highPriorityID)
	}
}

func TestVIRIDOptionalWildcardCatchAllHostAndVersion(t *testing.T) {
	router := NewVIRIDRouter()
	blogID := mustAddVIRIDRoute(t, router, methodGET, "/blog/:year/:month?", VIRIDRouteOptions{})
	wildcardID := mustAddVIRIDRoute(t, router, methodGET, "/health/?", VIRIDRouteOptions{})
	catchAllID := mustAddVIRIDRoute(t, router, methodGET, "/files/*path", VIRIDRouteOptions{})
	genericID := mustAddVIRIDRoute(t, router, methodGET, "/api/:id", VIRIDRouteOptions{})
	constrainedID := mustAddVIRIDRoute(t, router, methodGET, "/api/:id", VIRIDRouteOptions{
		Host:    "API.EXAMPLE.COM",
		Version: "v2",
	})

	cases := []struct {
		request VIRIDRequest
		wantID  uint64
		params  map[string]string
	}{
		{VIRIDRequest{MethodBit: methodGET, Path: "/blog/2026"}, blogID, map[string]string{"year": "2026"}},
		{VIRIDRequest{MethodBit: methodGET, Path: "/blog/2026/07"}, blogID, map[string]string{"year": "2026", "month": "07"}},
		{VIRIDRequest{MethodBit: methodGET, Path: "/health/live"}, wildcardID, map[string]string{}},
		{VIRIDRequest{MethodBit: methodGET, Path: "/files"}, catchAllID, map[string]string{"path": ""}},
		{VIRIDRequest{MethodBit: methodGET, Path: "/files/css/site.css"}, catchAllID, map[string]string{"path": "css/site.css"}},
		{VIRIDRequest{MethodBit: methodGET, Path: "/api/42"}, genericID, map[string]string{"id": "42"}},
		{VIRIDRequest{MethodBit: methodGET, Host: "api.example.com", Version: "v2", Path: "/api/42"}, constrainedID, map[string]string{"id": "42"}},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s/%s", tc.request.Host, tc.request.Version, tc.request.Path), func(t *testing.T) {
			match := router.Lookup(tc.request)
			if !match.Found || match.RouteID != tc.wantID {
				t.Fatalf("match=%+v, want route %d", match, tc.wantID)
			}
			if !reflect.DeepEqual(match.Params, tc.params) {
				t.Fatalf("params=%v, want %v", match.Params, tc.params)
			}
		})
	}
}

func TestVIRIDBasisLimitFallsBackWithoutChangingResults(t *testing.T) {
	router := NewVIRIDRouterWithOptions(VIRIDOptions{MaxBasisGenerators: 8})
	_ = mustAddVIRIDRoute(t, router, methodGET, "/users", VIRIDRouteOptions{})
	wantID := mustAddVIRIDRoute(t, router, methodGET, "/users/:id", VIRIDRouteOptions{})
	_ = mustAddVIRIDRoute(t, router, methodGET, "/*rest", VIRIDRouteOptions{})

	err := router.Compact()
	if !errors.Is(err, ErrVIRIDBasisLimit) {
		t.Fatalf("Compact error=%v, want ErrVIRIDBasisLimit", err)
	}
	stats := router.Stats()
	if stats.FallbackGenerations != 1 || stats.CompiledGenerations != 0 {
		t.Fatalf("unexpected fallback stats: %+v", stats)
	}
	match := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: "/users/42"})
	if !match.Found || match.RouteID != wantID || match.Params["id"] != "42" {
		t.Fatalf("fallback match=%+v", match)
	}
	trace := router.Explain(VIRIDRequest{MethodBit: methodGET, Path: "/users/42"})
	if !trace.FallbackUsed || !trace.Generations[0].Fallback {
		t.Fatalf("fallback was not visible in trace: %+v", trace)
	}
}

func TestVIRIDDeltaDeleteAndCompaction(t *testing.T) {
	router := NewVIRIDRouter()
	paramID := mustAddVIRIDRoute(t, router, methodGET, "/users/:id", VIRIDRouteOptions{})
	staticID := mustAddVIRIDRoute(t, router, methodGET, "/users/current", VIRIDRouteOptions{})

	if got := router.Stats().Generations; got != 2 {
		t.Fatalf("delta generations=%d, want 2", got)
	}
	if match := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: "/users/current"}); match.RouteID != staticID {
		t.Fatalf("pre-delete winner=%d, want %d", match.RouteID, staticID)
	}
	if !router.DeleteRoute(staticID) {
		t.Fatal("DeleteRoute returned false")
	}
	if router.DeleteRoute(staticID) {
		t.Fatal("second DeleteRoute should return false")
	}
	if match := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: "/users/current"}); match.RouteID != paramID {
		t.Fatalf("post-delete winner=%d, want %d", match.RouteID, paramID)
	}
	if got := router.Stats().Tombstones; got != 1 {
		t.Fatalf("tombstones=%d, want 1", got)
	}
	if err := router.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	stats := router.Stats()
	if stats.Generations != 1 || stats.Routes != 1 || stats.Tombstones != 0 {
		t.Fatalf("unexpected compacted stats: %+v", stats)
	}
}

func TestVIRIDDifferentialAgainstIndependentLinearOracle(t *testing.T) {
	router := NewVIRIDRouter()
	definitions := []viridOracleRoute{
		{method: methodGET, pattern: "/"},
		{method: methodGET, pattern: "/users"},
		{method: methodGET, pattern: "/users/:id"},
		{method: methodGET, pattern: "/assets/*"},
		{method: methodGET, pattern: "/*"},
		{method: methodPOST, pattern: "/users/:id"},
	}
	for index := range definitions {
		definitions[index].id = mustAddVIRIDRoute(t, router, definitions[index].method, definitions[index].pattern, VIRIDRouteOptions{})
		definitions[index].order = index
	}

	random := rand.New(rand.NewSource(20260714))
	paths := make([]string, 0, 200)
	vocabulary := []string{"users", "assets", "42", "css", "missing", "current"}
	for range 200 {
		length := random.Intn(5)
		parts := make([]string, length)
		for i := range parts {
			parts[i] = vocabulary[random.Intn(len(vocabulary))]
		}
		paths = append(paths, "/"+strings.Join(parts, "/"))
	}

	assertDifferential := func(phase string) {
		t.Helper()
		for index, path := range paths {
			method := methodGET
			if index%5 == 0 {
				method = methodPOST
			}
			wantID, wantFound := viridOracleLookup(definitions, method, normalizePattern(path))
			got := router.Lookup(VIRIDRequest{MethodBit: method, Path: path})
			if got.Found != wantFound || got.RouteID != wantID {
				t.Fatalf("%s path=%q method=%d: got=%+v wantID=%d found=%v", phase, path, method, got, wantID, wantFound)
			}
		}
	}

	assertDifferential("delta")
	if err := router.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	assertDifferential("compacted")
}

func TestVIRIDConcurrentReadersAndSnapshotWriters(t *testing.T) {
	router := NewVIRIDRouter()
	_ = mustAddVIRIDRoute(t, router, methodGET, "/*rest", VIRIDRouteOptions{})

	var wait sync.WaitGroup
	for reader := 0; reader < 4; reader++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for i := 0; i < 500; i++ {
				match := router.Lookup(VIRIDRequest{MethodBit: methodGET, Path: fmt.Sprintf("/p/%d", i)})
				if !match.Found {
					t.Errorf("concurrent lookup %d did not match catch-all", i)
					return
				}
			}
		}()
	}
	for i := 0; i < 20; i++ {
		id := mustAddVIRIDRoute(t, router, methodGET, fmt.Sprintf("/static/%d", i), VIRIDRouteOptions{})
		if i%2 == 0 && !router.DeleteRoute(id) {
			t.Fatalf("DeleteRoute(%d) failed", id)
		}
	}
	wait.Wait()
}

func TestVIRIDRejectsInvalidPatterns(t *testing.T) {
	router := NewVIRIDRouter()
	if err := router.AddRoute(methodGET, "/files/*rest/more", nil); err == nil {
		t.Fatal("expected non-suffix catch-all error")
	}
	if err := router.AddRoute(methodGET, "/users/:", nil); err == nil {
		t.Fatal("expected empty parameter error")
	}
	pattern := "/"
	for i := 0; i < maxVIRIDOptionalSegments+1; i++ {
		pattern += fmt.Sprintf("s%d?/", i)
	}
	if err := router.AddRoute(methodGET, pattern, nil); err == nil {
		t.Fatal("expected optional expansion limit error")
	}
}

func mustAddVIRIDRoute(
	t *testing.T,
	router *VIRIDRouter,
	method int,
	pattern string,
	options VIRIDRouteOptions,
) uint64 {
	t.Helper()
	id, err := router.AddRouteWithOptions(method, pattern, []HandlerFunc{func(*Context) {}}, options)
	if err != nil {
		t.Fatalf("AddRouteWithOptions(%q) failed: %v", pattern, err)
	}
	return id
}

type viridOracleRoute struct {
	id      uint64
	method  int
	pattern string
	order   int
}

func viridOracleLookup(routes []viridOracleRoute, method int, path string) (uint64, bool) {
	pathParts := splitOraclePath(path)
	bestIndex := -1
	bestStatic := -1
	bestCatchAll := true
	bestParams := -1
	for index, route := range routes {
		if route.method != method {
			continue
		}
		parts := splitOraclePath(route.pattern)
		catchAll := len(parts) > 0 && parts[len(parts)-1] == "*"
		prefixLength := len(parts)
		if catchAll {
			prefixLength--
			if len(pathParts) < prefixLength {
				continue
			}
		} else if len(pathParts) != prefixLength {
			continue
		}
		static, params := 0, 0
		matched := true
		for position := 0; position < prefixLength; position++ {
			if strings.HasPrefix(parts[position], ":") {
				params++
				continue
			}
			if parts[position] != pathParts[position] {
				matched = false
				break
			}
			static++
		}
		if !matched {
			continue
		}
		if bestIndex == -1 ||
			static > bestStatic ||
			(static == bestStatic && bestCatchAll && !catchAll) ||
			(static == bestStatic && catchAll == bestCatchAll && params < bestParams) ||
			(static == bestStatic && catchAll == bestCatchAll && params == bestParams && route.order < routes[bestIndex].order) {
			bestIndex = index
			bestStatic = static
			bestCatchAll = catchAll
			bestParams = params
		}
	}
	if bestIndex == -1 {
		return 0, false
	}
	return routes[bestIndex].id, true
}

func splitOraclePath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
