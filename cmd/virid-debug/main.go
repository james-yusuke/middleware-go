package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"middleware-go/middleware"
	"os"
)

type debugReport struct {
	RouteIDs      map[string]uint64     `json:"route_ids"`
	BeforeCompact middleware.VIRIDStats `json:"before_compact"`
	CompactError  string                `json:"compact_error,omitempty"`
	AfterCompact  middleware.VIRIDStats `json:"after_compact"`
	Match         middleware.VIRIDMatch `json:"match"`
	Trace         middleware.VIRIDTrace `json:"trace"`
}

func main() {
	path := flag.String("path", "/users/42", "canonical request path to inspect")
	method := flag.String("method", "GET", "HTTP method")
	host := flag.String("host", "", "optional request host")
	version := flag.String("version", "", "optional API version")
	compact := flag.Bool("compact", true, "compact delta generations before lookup")
	maxBasis := flag.Int("max-basis", 262_144, "maximum exact product-ideal generators")
	flag.Parse()

	methodBit, err := middleware.HTTPMethodBit(*method)
	if err != nil {
		fatal(err)
	}
	router := middleware.NewVIRIDRouterWithOptions(middleware.VIRIDOptions{
		MaxBasisGenerators: *maxBasis,
	})
	routeIDs := map[string]uint64{}
	add := func(name, pattern string, options middleware.VIRIDRouteOptions) {
		id, addErr := router.AddRouteWithOptions(methodBit, pattern, nil, options)
		if addErr != nil {
			fatal(addErr)
		}
		routeIDs[name] = id
	}

	add("root", "/", middleware.VIRIDRouteOptions{})
	add("users", "/users", middleware.VIRIDRouteOptions{})
	add("user", "/users/:id", middleware.VIRIDRouteOptions{})
	add("post", "/users/:id/posts/:postId", middleware.VIRIDRouteOptions{})
	add("assets", "/assets/*path", middleware.VIRIDRouteOptions{})
	add("blog", "/blog/:year/:month?", middleware.VIRIDRouteOptions{})
	add("api-v2-host", "/api/:id", middleware.VIRIDRouteOptions{
		Host:    "api.example.com",
		Version: "v2",
	})

	report := debugReport{
		RouteIDs:      routeIDs,
		BeforeCompact: router.Stats(),
	}
	if *compact {
		if compactErr := router.Compact(); compactErr != nil {
			report.CompactError = compactErr.Error()
		}
	}
	report.AfterCompact = router.Stats()
	request := middleware.VIRIDRequest{
		MethodBit: methodBit,
		Host:      *host,
		Version:   *version,
		Path:      *path,
	}
	report.Match = router.Lookup(request)
	report.Trace = router.Explain(request)

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "virid-debug: %v\n", err)
	os.Exit(1)
}
