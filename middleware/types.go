package middleware

import (
	"context"
	"net/http"
	"sync"
)

type HandlerFunc func(*Context)

type RouterMode int

const (
	ModeTrie RouterMode = iota
	ModeRegex
	ModeAhoCorasick
	ModeVIRID
)

const (
	methodGET = 1 << iota
	methodPOST
	methodPUT
	methodDELETE
	methodPATCH
	methodHEAD
	methodOPTIONS
)

var methodToBit = map[string]int{
	http.MethodGet:     methodGET,
	http.MethodPost:    methodPOST,
	http.MethodPut:     methodPUT,
	http.MethodDelete:  methodDELETE,
	http.MethodPatch:   methodPATCH,
	http.MethodHead:    methodHEAD,
	http.MethodOptions: methodOPTIONS,
}

type Router interface {
	AddRoute(methodBit int, pattern string, handlers []HandlerFunc) error
	Find(methodBit int, path string) (handlers []HandlerFunc, params map[string]string, found bool)
	Use(middleware HandlerFunc)
}

type Engine struct {
	router           Router
	mode             RouterMode
	globalMW         []HandlerFunc
	ctxPool          sync.Pool
	NotFound         HandlerFunc
	MethodNotAllowed HandlerFunc
}

type Context struct {
	W        http.ResponseWriter
	R        *http.Request
	Params   map[string]string
	handlers []HandlerFunc
	index    int
	engine   *Engine
	Keys     map[string]interface{}
	cancel   context.CancelFunc
}
