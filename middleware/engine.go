package middleware

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

func (e *Engine) acquireContext(w http.ResponseWriter, r *http.Request) *Context {
	v := e.ctxPool.Get()
	if v == nil {
		c := &Context{engine: e}
		c.W = w
		c.R = r
		c.index = -1
		return c
	}
	c := v.(*Context)
	c.W = w
	c.R = r
	c.Params = nil
	c.handlers = nil
	c.index = -1
	c.Keys = nil
	c.cancel = nil
	c.engine = e
	return c
}

func (e *Engine) releaseContext(c *Context) {
	c.W = nil
	c.R = nil
	c.Params = nil
	c.handlers = nil
	c.index = -1
	c.Keys = nil
	c.cancel = nil
	e.ctxPool.Put(c)
}

func New() *Engine {
	e := &Engine{
		router: NewTrieRouter(),
		mode:   ModeTrie,
		NotFound: func(c *Context) {
			http.NotFound(c.W, c.R)
		},
		MethodNotAllowed: func(c *Context) {
			http.Error(c.W, "Method Not Allowed", http.StatusMethodNotAllowed)
		},
	}
	e.ctxPool.New = func() interface{} {
		return &Context{engine: e, index: -1}
	}
	return e
}

func (e *Engine) Use(mw HandlerFunc) {
	e.globalMW = append(e.globalMW, mw)
}

func (e *Engine) SwitchRouter(mode RouterMode) error {
	if e.mode == mode {
		return nil
	}
	var r Router
	switch mode {
	case ModeTrie:
		r = NewTrieRouter()
	case ModeRegex:
		r = NewRegexRouter()
	case ModeAhoCorasick:
		r = NewAhoCorasickRouter()
	default:
		return errors.New("unknown router mode")
	}
	e.router = r
	e.mode = mode
	return nil
}

func (e *Engine) Handle(method string, pattern string, handlers ...HandlerFunc) error {
	bit, ok := methodToBit[strings.ToUpper(method)]
	if !ok {
		return fmt.Errorf("unsupported method %s", method)
	}
	return e.router.AddRoute(bit, normalizePattern(pattern), handlers)
}

func (e *Engine) GET(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodGet, pattern, handlers...)
}

func (e *Engine) POST(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodPost, pattern, handlers...)
}

func (e *Engine) PUT(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodPut, pattern, handlers...)
}

func (e *Engine) DELETE(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodDelete, pattern, handlers...)
}

func (e *Engine) PATCH(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodPatch, pattern, handlers...)
}

func (e *Engine) OPTIONS(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodOptions, pattern, handlers...)
}

func (e *Engine) HEAD(pattern string, handlers ...HandlerFunc) error {
	return e.Handle(http.MethodHead, pattern, handlers...)
}

type RouterGroup struct {
	prefix string
	engine *Engine
	mw     []HandlerFunc
}

func (e *Engine) Group(prefix string, mws ...HandlerFunc) *RouterGroup {
	return &RouterGroup{prefix: normalizePattern(prefix), engine: e, mw: mws}
}

func (g *RouterGroup) Handle(method string, pattern string, handlers ...HandlerFunc) error {
	p := g.prefix
	if strings.HasSuffix(p, "/") && strings.HasPrefix(pattern, "/") {
		p = p[:len(p)-1]
	}
	all := make([]HandlerFunc, 0, len(g.mw)+len(handlers))
	all = append(all, g.mw...)
	all = append(all, handlers...)
	return g.engine.Handle(method, p+pattern, all...)
}

func (g *RouterGroup) GET(pattern string, handlers ...HandlerFunc) error {
	return g.Handle(http.MethodGet, pattern, handlers...)
}

func (g *RouterGroup) POST(pattern string, handlers ...HandlerFunc) error {
	return g.Handle(http.MethodPost, pattern, handlers...)
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := e.acquireContext(w, r)
	defer func() {
		if c.cancel != nil {
			c.cancel()
		}
		e.releaseContext(c)
	}()
	methodBit := 0
	if b, ok := methodToBit[r.Method]; ok {
		methodBit = b
	} else {
		c.handlers = append(c.handlers, e.MethodNotAllowed)
		c.Next()
		return
	}
	p := r.URL.Path
	p = normalizePattern(p)
	handlers, params, found := e.router.Find(methodBit, p)
	if !found {
		e.NotFound(c)
		return
	}
	allHandlers := make([]HandlerFunc, 0, len(e.globalMW)+len(handlers))
	allHandlers = append(allHandlers, e.globalMW...)
	allHandlers = append(allHandlers, handlers...)
	c.handlers = allHandlers
	c.Params = params
	c.index = -1
	c.Next()
}

func normalizePattern(p string) string {
	if p == "" {
		return "/"
	}
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func Recover() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[recover] panic: %v", r)
				http.Error(c.W, "internal server error", http.StatusInternalServerError)
				c.Abort()
			}
		}()
		c.Next()
	}
}

func LimitBody(maxBytes int64) HandlerFunc {
	return func(c *Context) {
		c.R.Body = http.MaxBytesReader(c.W, c.R.Body, maxBytes)
		c.Next()
	}
}

func Timeout(d time.Duration) HandlerFunc {
	return func(c *Context) {
		ctx, cancel := context.WithTimeout(c.R.Context(), d)
		c.cancel = cancel
		c.R = c.R.WithContext(ctx)
		c.Next()
	}
}

func Logger(prefix string) HandlerFunc {
	return func(c *Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s %s %s", prefix, c.R.Method, c.R.URL.Path, time.Since(start))
	}
}
