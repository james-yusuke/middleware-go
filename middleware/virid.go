package middleware

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

var ErrVIRIDBasisLimit = errors.New("virid: product-ideal generator limit exceeded")

const (
	defaultVIRIDMaxBasisGenerators = 65_536
	maxVIRIDOptionalSegments       = 10
)

// VIRIDOptions controls the intentionally bounded research implementation.
// A generation that exceeds MaxBasisGenerators remains correct by switching to
// the linear oracle. AutoCompactGenerations == 0 disables automatic compaction.
type VIRIDOptions struct {
	MaxBasisGenerators     int
	AutoCompactGenerations int
}

type VIRIDRouteOptions struct {
	Host             string
	Version          string
	ExplicitPriority int
}

type VIRIDRequest struct {
	MethodBit int    `json:"method_bit"`
	Host      string `json:"host,omitempty"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path"`
}

type VIRIDMatch struct {
	RouteID  uint64            `json:"route_id,omitempty"`
	Pattern  string            `json:"pattern,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Handlers []HandlerFunc     `json:"-"`
	Found    bool              `json:"found"`
}

type VIRIDStats struct {
	Epoch               uint64 `json:"epoch"`
	Generations         int    `json:"generations"`
	CompiledGenerations int    `json:"compiled_generations"`
	FallbackGenerations int    `json:"fallback_generations"`
	Routes              int    `json:"routes"`
	Variants            int    `json:"variants"`
	BasisGenerators     int    `json:"basis_generators"`
	MaxBasisGenerators  int    `json:"max_basis_generators"`
	Tombstones          int    `json:"tombstones"`
}

type VIRIDGenerationTrace struct {
	Generation             int      `json:"generation"`
	Routes                 int      `json:"routes"`
	BasisGenerators        int      `json:"basis_generators"`
	SpecializedNonZero     int      `json:"specialized_non_zero"`
	GCDCoefficients        []uint64 `json:"gcd_coefficients,omitempty"`
	RootCodes              []uint64 `json:"root_codes,omitempty"`
	MatchingRouteIDs       []uint64 `json:"matching_route_ids,omitempty"`
	OracleMatchingRouteIDs []uint64 `json:"oracle_matching_route_ids,omitempty"`
	CandidateSetAgrees     bool     `json:"candidate_set_agrees_with_oracle"`
	Fallback               bool     `json:"fallback"`
	Error                  string   `json:"error,omitempty"`
}

type VIRIDTrace struct {
	Request              VIRIDRequest           `json:"request"`
	Epoch                uint64                 `json:"epoch"`
	Generations          []VIRIDGenerationTrace `json:"generations"`
	WinnerRouteID        uint64                 `json:"winner_route_id,omitempty"`
	WinnerPattern        string                 `json:"winner_pattern,omitempty"`
	FallbackUsed         bool                   `json:"fallback_used"`
	VerificationFailures int                    `json:"verification_failures"`
}

type viridSegmentKind uint8

const (
	viridStaticSegment viridSegmentKind = iota
	viridParameterSegment
	viridWildcardSegment
	viridCatchAllSegment
)

type viridSegment struct {
	kind     viridSegmentKind
	literal  string
	name     string
	optional bool
}

type viridSpecificity struct {
	staticConstraints int
	exactArity        bool
	catchAlls         int
	wildcards         int
	parameters        int
	depth             int
}

type viridConstraintKind uint8

const (
	viridMethodConstraint viridConstraintKind = iota
	viridHostConstraint
	viridVersionConstraint
	viridStaticConstraint
	viridExactArityConstraint
	viridMinimumArityConstraint
)

// A constraint evaluates to zero when it is satisfied and to a field unit
// when it is not. This is the Boolean-predicate frontend of the polynomial
// f_r,k described in the VIRID report.
type viridConstraint struct {
	kind     viridConstraintKind
	position int
	integer  int
	literal  string
}

type viridRouteVariant struct {
	routeID          uint64
	localCode        uint64
	methodBit        int
	pattern          string
	host             string
	version          string
	explicitPriority int
	segments         []viridSegment
	constraints      []viridConstraint
	handlers         []HandlerFunc
	specificity      viridSpecificity
}

type viridFactorKind uint8

const (
	viridRouteCodeFactor viridFactorKind = iota
	viridConstraintFactor
)

type viridIdealFactor struct {
	kind       viridFactorKind
	routeCode  uint64
	constraint viridConstraint
}

// viridProductGenerator is a factored symbolic polynomial. Each generator in
// the product ideal chooses exactly one generator from each route ideal.
type viridProductGenerator struct {
	factors []viridIdealFactor
}

type viridGeneration struct {
	routes         []*viridRouteVariant
	basis          []viridProductGenerator
	fallback       bool
	compileMessage string
}

type viridSnapshot struct {
	epoch       uint64
	generations []*viridGeneration
	deleted     map[uint64]struct{}
	middlewares []HandlerFunc
}

type VIRIDRouter struct {
	mu          sync.Mutex
	snapshot    atomic.Pointer[viridSnapshot]
	nextRouteID uint64
	options     VIRIDOptions
}

func NewVIRIDRouter() *VIRIDRouter {
	return NewVIRIDRouterWithOptions(VIRIDOptions{})
}

func NewVIRIDRouterWithOptions(options VIRIDOptions) *VIRIDRouter {
	if options.MaxBasisGenerators <= 0 {
		options.MaxBasisGenerators = defaultVIRIDMaxBasisGenerators
	}
	router := &VIRIDRouter{options: options}
	router.snapshot.Store(&viridSnapshot{deleted: map[uint64]struct{}{}})
	return router
}

// HTTPMethodBit exposes the Engine's method encoding to research tools without
// making callers duplicate its private constants.
func HTTPMethodBit(method string) (int, error) {
	bit, ok := methodToBit[strings.ToUpper(method)]
	if !ok {
		return 0, fmt.Errorf("unsupported method %s", method)
	}
	return bit, nil
}

func (r *VIRIDRouter) Use(middleware HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.snapshot.Load()
	next := cloneVIRIDSnapshot(current)
	next.epoch++
	next.middlewares = append(slices.Clone(current.middlewares), middleware)
	r.snapshot.Store(next)
}

func (r *VIRIDRouter) AddRoute(methodBit int, pattern string, handlers []HandlerFunc) error {
	_, err := r.AddRouteWithOptions(methodBit, pattern, handlers, VIRIDRouteOptions{})
	return err
}

func (r *VIRIDRouter) AddRouteWithOptions(
	methodBit int,
	pattern string,
	handlers []HandlerFunc,
	options VIRIDRouteOptions,
) (uint64, error) {
	if methodBit == 0 {
		return 0, errors.New("virid: method bit must be non-zero")
	}
	normalized := normalizePattern(pattern)
	segmentVariants, err := parseVIRIDPattern(normalized)
	if err != nil {
		return 0, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextRouteID++
	routeID := r.nextRouteID
	variants := make([]*viridRouteVariant, 0, len(segmentVariants))
	for _, segments := range segmentVariants {
		variant := buildVIRIDRouteVariant(
			routeID,
			methodBit,
			normalized,
			segments,
			handlers,
			options,
		)
		variants = append(variants, variant)
	}

	generation, _ := compileVIRIDGeneration(variants, r.options.MaxBasisGenerators)
	current := r.snapshot.Load()
	next := cloneVIRIDSnapshot(current)
	next.epoch++
	next.generations = append(slices.Clone(current.generations), generation)

	if threshold := r.options.AutoCompactGenerations; threshold > 0 && len(next.generations) >= threshold {
		next, _ = r.compactSnapshot(next)
	}
	r.snapshot.Store(next)
	return routeID, nil
}

// DeleteRoute publishes a logical tombstone. Existing immutable bases are not
// modified and are reclaimed by Compact.
func (r *VIRIDRouter) DeleteRoute(routeID uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.snapshot.Load()
	if _, deleted := current.deleted[routeID]; deleted || !snapshotHasVIRIDRoute(current, routeID) {
		return false
	}
	next := cloneVIRIDSnapshot(current)
	next.epoch++
	next.deleted = cloneVIRIDTombstones(current.deleted)
	next.deleted[routeID] = struct{}{}
	r.snapshot.Store(next)
	return true
}

// Compact merges all visible generations. If the exact product basis exceeds
// the configured cap, the newly published generation is marked as a fallback
// generation and lookup remains correct.
func (r *VIRIDRouter) Compact() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next, err := r.compactSnapshot(r.snapshot.Load())
	r.snapshot.Store(next)
	return err
}

func (r *VIRIDRouter) compactSnapshot(current *viridSnapshot) (*viridSnapshot, error) {
	active := make([]*viridRouteVariant, 0)
	for _, generation := range current.generations {
		for _, route := range generation.routes {
			if _, deleted := current.deleted[route.routeID]; deleted {
				continue
			}
			active = append(active, route)
		}
	}

	next := cloneVIRIDSnapshot(current)
	next.epoch++
	next.deleted = map[uint64]struct{}{}
	if len(active) == 0 {
		next.generations = nil
		return next, nil
	}
	generation, err := compileVIRIDGeneration(active, r.options.MaxBasisGenerators)
	next.generations = []*viridGeneration{generation}
	return next, err
}

func (r *VIRIDRouter) Find(methodBit int, path string) ([]HandlerFunc, map[string]string, bool) {
	match, snapshot, _ := r.lookup(VIRIDRequest{MethodBit: methodBit, Path: path}, false)
	if !match.Found {
		return nil, nil, false
	}
	handlers := make([]HandlerFunc, 0, len(snapshot.middlewares)+len(match.Handlers))
	handlers = append(handlers, snapshot.middlewares...)
	handlers = append(handlers, match.Handlers...)
	return handlers, match.Params, true
}

func (r *VIRIDRouter) Lookup(request VIRIDRequest) VIRIDMatch {
	match, _, _ := r.lookup(request, false)
	return match
}

func (r *VIRIDRouter) Explain(request VIRIDRequest) VIRIDTrace {
	_, _, trace := r.lookup(request, true)
	return trace
}

func (r *VIRIDRouter) lookup(request VIRIDRequest, wantTrace bool) (VIRIDMatch, *viridSnapshot, VIRIDTrace) {
	request.Path = normalizePattern(request.Path)
	request.Host = strings.ToLower(request.Host)
	query := viridQuery{
		methodBit: request.MethodBit,
		host:      request.Host,
		version:   request.Version,
		segments:  splitVIRIDPath(request.Path),
	}
	snapshot := r.snapshot.Load()
	trace := VIRIDTrace{Request: request, Epoch: snapshot.epoch}
	var best *viridRouteVariant
	var bestParams map[string]string

	for generationIndex, generation := range snapshot.generations {
		generationTrace := VIRIDGenerationTrace{
			Generation:      generationIndex,
			Routes:          len(generation.routes),
			BasisGenerators: len(generation.basis),
			Fallback:        generation.fallback,
		}

		var candidates []*viridRouteVariant
		if generation.fallback {
			trace.FallbackUsed = true
			generationTrace.Error = generation.compileMessage
			candidates = generation.routes
		} else {
			decoded, err := decodeVIRIDGeneration(generation, query, &generationTrace)
			if err != nil {
				trace.FallbackUsed = true
				generationTrace.Fallback = true
				generationTrace.Error = err.Error()
				candidates = generation.routes
			} else {
				candidates = decoded
			}
		}

		for _, candidate := range candidates {
			if _, deleted := snapshot.deleted[candidate.routeID]; deleted {
				continue
			}
			params, matches := verifyVIRIDRoute(candidate, query)
			if !matches {
				// A fallback generation scans non-matches by design. A decoded
				// algebraic candidate failing verification is noteworthy.
				if !generationTrace.Fallback {
					trace.VerificationFailures++
				}
				continue
			}
			if best == nil || higherPriorityVIRID(candidate, best) {
				best = candidate
				bestParams = params
			}
		}
		if wantTrace {
			for _, route := range generation.routes {
				if _, matches := verifyVIRIDRoute(route, query); matches {
					generationTrace.OracleMatchingRouteIDs = append(
						generationTrace.OracleMatchingRouteIDs,
						route.routeID,
					)
				}
			}
			if generationTrace.Fallback {
				generationTrace.MatchingRouteIDs = slices.Clone(generationTrace.OracleMatchingRouteIDs)
			}
			generationTrace.CandidateSetAgrees = slices.Equal(
				generationTrace.MatchingRouteIDs,
				generationTrace.OracleMatchingRouteIDs,
			)
			trace.Generations = append(trace.Generations, generationTrace)
		}
	}

	if best == nil {
		return VIRIDMatch{}, snapshot, trace
	}
	trace.WinnerRouteID = best.routeID
	trace.WinnerPattern = best.pattern
	return VIRIDMatch{
		RouteID:  best.routeID,
		Pattern:  best.pattern,
		Params:   bestParams,
		Handlers: slices.Clone(best.handlers),
		Found:    true,
	}, snapshot, trace
}

func (r *VIRIDRouter) Stats() VIRIDStats {
	snapshot := r.snapshot.Load()
	stats := VIRIDStats{
		Epoch:              snapshot.epoch,
		Generations:        len(snapshot.generations),
		MaxBasisGenerators: r.options.MaxBasisGenerators,
		Tombstones:         len(snapshot.deleted),
	}
	routeIDs := map[uint64]struct{}{}
	for _, generation := range snapshot.generations {
		if generation.fallback {
			stats.FallbackGenerations++
		} else {
			stats.CompiledGenerations++
		}
		stats.BasisGenerators += len(generation.basis)
		for _, route := range generation.routes {
			if _, deleted := snapshot.deleted[route.routeID]; deleted {
				continue
			}
			stats.Variants++
			routeIDs[route.routeID] = struct{}{}
		}
	}
	stats.Routes = len(routeIDs)
	return stats
}

type viridQuery struct {
	methodBit int
	host      string
	version   string
	segments  []string
}

func (constraint viridConstraint) evaluate(query viridQuery) uint64 {
	matched := false
	switch constraint.kind {
	case viridMethodConstraint:
		matched = query.methodBit == constraint.integer
	case viridHostConstraint:
		matched = query.host == constraint.literal
	case viridVersionConstraint:
		matched = query.version == constraint.literal
	case viridStaticConstraint:
		matched = constraint.position < len(query.segments) &&
			query.segments[constraint.position] == constraint.literal
	case viridExactArityConstraint:
		matched = len(query.segments) == constraint.integer
	case viridMinimumArityConstraint:
		matched = len(query.segments) >= constraint.integer
	default:
		panic("virid: unknown constraint kind")
	}
	if matched {
		return 0
	}
	return 1
}

func compileVIRIDGeneration(routes []*viridRouteVariant, maxGenerators int) (*viridGeneration, error) {
	compiledRoutes := make([]*viridRouteVariant, len(routes))
	for index, route := range routes {
		clone := *route
		clone.localCode = uint64(index + 1)
		clone.segments = slices.Clone(route.segments)
		clone.constraints = slices.Clone(route.constraints)
		clone.handlers = slices.Clone(route.handlers)
		compiledRoutes[index] = &clone
	}
	if len(compiledRoutes) >= int(viridPrime) {
		generation := &viridGeneration{routes: compiledRoutes, fallback: true}
		generation.compileMessage = "route-code space exhausted"
		return generation, fmt.Errorf("virid: route-code space exhausted")
	}

	basis := []viridProductGenerator{{}}
	for _, route := range compiledRoutes {
		ideal := make([]viridIdealFactor, 0, 1+len(route.constraints))
		ideal = append(ideal, viridIdealFactor{
			kind:      viridRouteCodeFactor,
			routeCode: route.localCode,
		})
		for _, constraint := range route.constraints {
			ideal = append(ideal, viridIdealFactor{
				kind:       viridConstraintFactor,
				constraint: constraint,
			})
		}
		if len(ideal) > maxGenerators || len(basis) > maxGenerators/len(ideal) {
			detail := fmt.Sprintf(
				"after route %d: %d x %d generators exceeds cap %d",
				route.routeID,
				len(basis),
				len(ideal),
				maxGenerators,
			)
			message := fmt.Sprintf("%v %s", ErrVIRIDBasisLimit, detail)
			generation := &viridGeneration{
				routes:         compiledRoutes,
				fallback:       true,
				compileMessage: message,
			}
			return generation, fmt.Errorf("%w: %s", ErrVIRIDBasisLimit, detail)
		}

		next := make([]viridProductGenerator, 0, len(basis)*len(ideal))
		for _, existing := range basis {
			for _, factor := range ideal {
				factors := make([]viridIdealFactor, len(existing.factors), len(existing.factors)+1)
				copy(factors, existing.factors)
				factors = append(factors, factor)
				next = append(next, viridProductGenerator{factors: factors})
			}
		}
		basis = next
	}
	return &viridGeneration{routes: compiledRoutes, basis: basis}, nil
}

func decodeVIRIDGeneration(
	generation *viridGeneration,
	query viridQuery,
	trace *VIRIDGenerationTrace,
) ([]*viridRouteVariant, error) {
	polynomials := make([]viridPoly, 0, len(generation.basis))
	for _, generator := range generation.basis {
		polynomial, nonZero := specializeVIRIDGenerator(generator, query)
		if !nonZero {
			continue
		}
		polynomials = append(polynomials, polynomial)
	}
	trace.SpecializedNonZero = len(polynomials)
	if len(polynomials) == 0 {
		return nil, errors.New("virid: all specialized generators vanished")
	}
	winnerPolynomial := viridPolyGCDAll(polynomials)
	if len(winnerPolynomial) == 0 {
		return nil, errors.New("virid: specialized ideal produced zero GCD")
	}
	trace.GCDCoefficients = append([]uint64(nil), winnerPolynomial...)

	roots := make([]uint64, 0, viridPolyDegree(winnerPolynomial))
	candidates := make([]*viridRouteVariant, 0, cap(roots))
	for _, route := range generation.routes {
		if viridPolyEval(winnerPolynomial, route.localCode) != 0 {
			continue
		}
		roots = append(roots, route.localCode)
		candidates = append(candidates, route)
		trace.MatchingRouteIDs = append(trace.MatchingRouteIDs, route.routeID)
	}
	trace.RootCodes = roots
	if len(roots) != viridPolyDegree(winnerPolynomial) {
		return nil, fmt.Errorf(
			"virid: root decode found %d known roots for degree %d polynomial",
			len(roots),
			viridPolyDegree(winnerPolynomial),
		)
	}
	return candidates, nil
}

func specializeVIRIDGenerator(generator viridProductGenerator, query viridQuery) (viridPoly, bool) {
	polynomial := viridPoly{1}
	for _, factor := range generator.factors {
		switch factor.kind {
		case viridRouteCodeFactor:
			polynomial = viridPolyMulLinear(polynomial, factor.routeCode)
		case viridConstraintFactor:
			value := factor.constraint.evaluate(query)
			if value == 0 {
				return nil, false
			}
			polynomial = viridPolyScale(polynomial, value)
		default:
			panic("virid: unknown ideal factor")
		}
	}
	return polynomial, len(polynomial) > 0
}

func parseVIRIDPattern(pattern string) ([][]viridSegment, error) {
	parts := splitVIRIDPath(pattern)
	segments := make([]viridSegment, 0, len(parts))
	optionalCount := 0
	for index, part := range parts {
		segment := viridSegment{}
		if part != "?" && strings.HasSuffix(part, "?") {
			segment.optional = true
			part = strings.TrimSuffix(part, "?")
			optionalCount++
			if part == "" {
				return nil, errors.New("virid: empty optional segment")
			}
		}

		switch {
		case part == "?":
			segment.kind = viridWildcardSegment
		case strings.HasPrefix(part, "*"):
			if index != len(parts)-1 {
				return nil, errors.New("virid: catch-all must be the final segment")
			}
			if segment.optional {
				return nil, errors.New("virid: catch-all already matches an empty suffix and cannot be optional")
			}
			segment.kind = viridCatchAllSegment
			segment.name = strings.TrimPrefix(part, "*")
			if segment.name == "" {
				segment.name = "*"
			}
		case strings.HasPrefix(part, ":"):
			segment.kind = viridParameterSegment
			segment.name = strings.TrimPrefix(part, ":")
			if segment.name == "" {
				return nil, errors.New("virid: parameter name cannot be empty")
			}
		default:
			segment.kind = viridStaticSegment
			segment.literal = part
		}
		segments = append(segments, segment)
	}
	if optionalCount > maxVIRIDOptionalSegments {
		return nil, fmt.Errorf("virid: at most %d optional segments are supported", maxVIRIDOptionalSegments)
	}

	variants := [][]viridSegment{{}}
	for _, segment := range segments {
		if !segment.optional {
			for i := range variants {
				variants[i] = append(variants[i], segment)
			}
			continue
		}
		next := make([][]viridSegment, 0, len(variants)*2)
		segment.optional = false
		for _, variant := range variants {
			present := append(slices.Clone(variant), segment)
			absent := slices.Clone(variant)
			next = append(next, present, absent)
		}
		variants = next
	}
	return variants, nil
}

func buildVIRIDRouteVariant(
	routeID uint64,
	methodBit int,
	pattern string,
	segments []viridSegment,
	handlers []HandlerFunc,
	options VIRIDRouteOptions,
) *viridRouteVariant {
	host := strings.ToLower(options.Host)
	variant := &viridRouteVariant{
		routeID:          routeID,
		methodBit:        methodBit,
		pattern:          pattern,
		host:             host,
		version:          options.Version,
		explicitPriority: options.ExplicitPriority,
		segments:         slices.Clone(segments),
		handlers:         slices.Clone(handlers),
	}
	variant.constraints = append(variant.constraints, viridConstraint{
		kind:    viridMethodConstraint,
		integer: methodBit,
	})
	if host != "" {
		variant.constraints = append(variant.constraints, viridConstraint{
			kind:    viridHostConstraint,
			literal: host,
		})
		variant.specificity.staticConstraints++
	}
	if options.Version != "" {
		variant.constraints = append(variant.constraints, viridConstraint{
			kind:    viridVersionConstraint,
			literal: options.Version,
		})
		variant.specificity.staticConstraints++
	}

	catchAll := false
	for position, segment := range segments {
		switch segment.kind {
		case viridStaticSegment:
			variant.constraints = append(variant.constraints, viridConstraint{
				kind:     viridStaticConstraint,
				position: position,
				literal:  segment.literal,
			})
			variant.specificity.staticConstraints++
		case viridParameterSegment:
			variant.specificity.parameters++
		case viridWildcardSegment:
			variant.specificity.wildcards++
		case viridCatchAllSegment:
			variant.specificity.catchAlls++
			catchAll = true
		}
	}
	variant.specificity.depth = len(segments)
	if catchAll {
		variant.constraints = append(variant.constraints, viridConstraint{
			kind:    viridMinimumArityConstraint,
			integer: len(segments) - 1,
		})
	} else {
		variant.constraints = append(variant.constraints, viridConstraint{
			kind:    viridExactArityConstraint,
			integer: len(segments),
		})
		variant.specificity.exactArity = true
	}
	return variant
}

func verifyVIRIDRoute(route *viridRouteVariant, query viridQuery) (map[string]string, bool) {
	if route.methodBit != query.methodBit ||
		(route.host != "" && route.host != query.host) ||
		(route.version != "" && route.version != query.version) {
		return nil, false
	}
	catchAll := len(route.segments) > 0 && route.segments[len(route.segments)-1].kind == viridCatchAllSegment
	prefixLength := len(route.segments)
	if catchAll {
		prefixLength--
		if len(query.segments) < prefixLength {
			return nil, false
		}
	} else if len(query.segments) != prefixLength {
		return nil, false
	}

	params := map[string]string{}
	for position := 0; position < prefixLength; position++ {
		segment := route.segments[position]
		value := query.segments[position]
		switch segment.kind {
		case viridStaticSegment:
			if value != segment.literal {
				return nil, false
			}
		case viridParameterSegment:
			params[segment.name] = value
		case viridWildcardSegment:
			// A one-segment wildcard deliberately does not capture.
		default:
			return nil, false
		}
	}
	if catchAll {
		segment := route.segments[len(route.segments)-1]
		params[segment.name] = strings.Join(query.segments[prefixLength:], "/")
	}
	return params, true
}

func higherPriorityVIRID(candidate, incumbent *viridRouteVariant) bool {
	if candidate.explicitPriority != incumbent.explicitPriority {
		return candidate.explicitPriority > incumbent.explicitPriority
	}
	cs, is := candidate.specificity, incumbent.specificity
	if cs.staticConstraints != is.staticConstraints {
		return cs.staticConstraints > is.staticConstraints
	}
	if cs.exactArity != is.exactArity {
		return cs.exactArity
	}
	if cs.catchAlls != is.catchAlls {
		return cs.catchAlls < is.catchAlls
	}
	if cs.wildcards != is.wildcards {
		return cs.wildcards < is.wildcards
	}
	if cs.parameters != is.parameters {
		return cs.parameters < is.parameters
	}
	if cs.depth != is.depth {
		return cs.depth > is.depth
	}
	return candidate.routeID < incumbent.routeID
}

func splitVIRIDPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func cloneVIRIDSnapshot(snapshot *viridSnapshot) *viridSnapshot {
	return &viridSnapshot{
		epoch:       snapshot.epoch,
		generations: slices.Clone(snapshot.generations),
		deleted:     snapshot.deleted,
		middlewares: slices.Clone(snapshot.middlewares),
	}
}

func cloneVIRIDTombstones(source map[uint64]struct{}) map[uint64]struct{} {
	clone := make(map[uint64]struct{}, len(source)+1)
	for routeID := range source {
		clone[routeID] = struct{}{}
	}
	return clone
}

func snapshotHasVIRIDRoute(snapshot *viridSnapshot, routeID uint64) bool {
	for _, generation := range snapshot.generations {
		for _, route := range generation.routes {
			if route.routeID == routeID {
				return true
			}
		}
	}
	return false
}

var _ Router = (*VIRIDRouter)(nil)
