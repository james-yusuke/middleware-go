package middleware

import (
	"strings"
	"testing"
)

func TestRegexRouterRejectsTooLongPattern(t *testing.T) {
	r := NewRegexRouter()
	pattern := "/" + strings.Repeat("a", maxRegexPatternLen)
	if err := r.AddRoute(methodGET, pattern, []HandlerFunc{mark("too-long")}); err == nil {
		t.Fatal("expected a too-long pattern to be rejected")
	}
}

func TestRegexRouterSpecificityDoesNotDependOnRegistrationOrder(t *testing.T) {
	r := NewRegexRouter()
	mustAddRoute(t, r, methodGET, "/things/:id", mark("param"))
	mustAddRoute(t, r, methodGET, "/things/new", mark("static"))

	handlers, _, found := r.Find(methodGET, "/things/new")
	if !found {
		t.Fatal("expected route to be found")
	}
	assertSeen(t, runHandlerChain(handlers), []string{"static"})
}
