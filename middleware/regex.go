package middleware

import (
	"fmt"
	"regexp"
	"strings"
)

type regexRoute struct {
	methodBit   int
	pattern     string
	re          *regexp.Regexp
	names       []string
	handlers    []HandlerFunc
	specificity routeSpecificity
	order       int
}

type routeSpecificity struct {
	staticSegments int
	paramSegments  int
	wildcards      int
	depth          int
}

type RegexRouter struct {
	routes      []regexRoute
	middlewares []HandlerFunc
}

const maxRegexPatternLen = 2048

var nestedQuantifier = regexp.MustCompile(`\(\?:?.*[\+\*\{].*\).*[+\*\{]`)

func NewRegexRouter() *RegexRouter {
	return &RegexRouter{}
}

func (rr *RegexRouter) Use(m HandlerFunc) {
	rr.middlewares = append(rr.middlewares, m)
}

func (rr *RegexRouter) AddRoute(methodBit int, pattern string, handlers []HandlerFunc) error {
	if len(pattern) > maxRegexPatternLen {
		return fmt.Errorf("pattern too long")
	}
	p := strings.Trim(pattern, "/")
	var segs []string
	if p == "" {
		segs = []string{}
	} else {
		segs = strings.Split(p, "/")
	}
	parts := make([]string, 0, len(segs))
	names := make([]string, 0)
	spec := routeSpecificity{depth: len(segs)}
	for _, s := range segs {
		if strings.HasPrefix(s, ":") {
			parts = append(parts, "([^/]+)")
			names = append(names, s[1:])
			spec.paramSegments++
		} else if s == "*" {
			parts = append(parts, "(.*)")
			names = append(names, "*")
			spec.wildcards++
		} else {
			parts = append(parts, regexp.QuoteMeta(s))
			spec.staticSegments++
		}
	}
	regexStr := "^/"
	if len(parts) > 0 {
		regexStr += strings.Join(parts, "/")
	}
	regexStr += "$"
	if nestedQuantifier.MatchString(regexStr) {
		return fmt.Errorf("pattern rejected: nested quantifier suspects")
	}
	re, err := regexp.Compile(regexStr)
	if err != nil {
		return err
	}
	rr.routes = append(rr.routes, regexRoute{
		methodBit:   methodBit,
		pattern:     pattern,
		re:          re,
		names:       names,
		handlers:    handlers,
		specificity: spec,
		order:       len(rr.routes),
	})
	return nil
}

func (rr *RegexRouter) Find(methodBit int, p string) ([]HandlerFunc, map[string]string, bool) {
	var best *regexRoute
	var bestMatch []string
	for i := range rr.routes {
		rt := &rr.routes[i]
		if rt.methodBit != methodBit {
			continue
		}
		m := rt.re.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		if best == nil || moreSpecific(rt, best) {
			best = rt
			bestMatch = m
		}
	}
	if best == nil {
		return nil, nil, false
	}
	params := map[string]string{}
	for i, name := range best.names {
		if 1+i < len(bestMatch) {
			params[name] = bestMatch[1+i]
		}
	}
	all := make([]HandlerFunc, 0, len(rr.middlewares)+len(best.handlers))
	all = append(all, rr.middlewares...)
	all = append(all, best.handlers...)
	return all, params, true
}

func moreSpecific(candidate, incumbent *regexRoute) bool {
	cs := candidate.specificity
	is := incumbent.specificity
	if cs.staticSegments != is.staticSegments {
		return cs.staticSegments > is.staticSegments
	}
	if cs.paramSegments != is.paramSegments {
		return cs.paramSegments > is.paramSegments
	}
	if cs.wildcards != is.wildcards {
		return cs.wildcards < is.wildcards
	}
	if cs.depth != is.depth {
		return cs.depth > is.depth
	}
	return candidate.order < incumbent.order
}
