package middleware

import "strings"

type trieNode struct {
	part          string
	children      map[string]*trieNode
	paramChild    *trieNode
	paramName     string
	wildcardChild *trieNode
	handlersByBit map[int][]HandlerFunc
}

type TrieRouter struct {
	root        *trieNode
	middlewares []HandlerFunc
}

func NewTrieRouter() *TrieRouter {
	return &TrieRouter{
		root: &trieNode{
			children:      map[string]*trieNode{},
			handlersByBit: map[int][]HandlerFunc{},
		},
	}
}

func (tr *TrieRouter) Use(m HandlerFunc) {
	tr.middlewares = append(tr.middlewares, m)
}

func (tr *TrieRouter) AddRoute(methodBit int, pattern string, handlers []HandlerFunc) error {
	p := strings.Trim(pattern, "/")
	var segs []string
	if p == "" {
		segs = []string{}
	} else {
		segs = strings.Split(p, "/")
	}
	node := tr.root
	for _, s := range segs {
		if strings.HasPrefix(s, ":") {
			if node.paramChild == nil {
				node.paramChild = &trieNode{
					children:      map[string]*trieNode{},
					handlersByBit: map[int][]HandlerFunc{},
				}
				node.paramChild.paramName = s[1:]
			}
			node = node.paramChild
			continue
		}
		if s == "*" {
			if node.wildcardChild == nil {
				node.wildcardChild = &trieNode{
					children:      map[string]*trieNode{},
					handlersByBit: map[int][]HandlerFunc{},
				}
			}
			node = node.wildcardChild
			break
		}
		child, ok := node.children[s]
		if !ok {
			child = &trieNode{
				part:          s,
				children:      map[string]*trieNode{},
				handlersByBit: map[int][]HandlerFunc{},
			}
			node.children[s] = child
		}
		node = child
	}
	if node.handlersByBit == nil {
		node.handlersByBit = map[int][]HandlerFunc{}
	}
	node.handlersByBit[methodBit] = handlers
	return nil
}

func (tr *TrieRouter) Find(methodBit int, p string) ([]HandlerFunc, map[string]string, bool) {
	p = strings.Trim(p, "/")
	var segs []string
	if p == "" {
		segs = []string{}
	} else {
		segs = strings.Split(p, "/")
	}
	params := make(map[string]string)
	node := tr.root
	for i := 0; i <= len(segs); i++ {
		if i == len(segs) {
			if node == nil {
				break
			}
			if handlers, ok := node.handlersByBit[methodBit]; ok && handlers != nil {
				all := make([]HandlerFunc, 0, len(tr.middlewares)+len(handlers))
				all = append(all, tr.middlewares...)
				all = append(all, handlers...)
				return all, params, true
			}
			if node.wildcardChild != nil {
				if handlers, ok := node.wildcardChild.handlersByBit[methodBit]; ok && handlers != nil {
					all := make([]HandlerFunc, 0, len(tr.middlewares)+len(handlers))
					all = append(all, tr.middlewares...)
					all = append(all, handlers...)
					return all, params, true
				}
			}
			break
		}
		seg := segs[i]
		if node == nil {
			break
		}
		if child, ok := node.children[seg]; ok {
			node = child
			continue
		}
		if node.paramChild != nil {
			params[node.paramChild.paramName] = seg
			node = node.paramChild
			continue
		}
		if node.wildcardChild != nil {
			params["*"] = strings.Join(segs[i:], "/")
			if handlers, ok := node.wildcardChild.handlersByBit[methodBit]; ok && handlers != nil {
				all := make([]HandlerFunc, 0, len(tr.middlewares)+len(handlers))
				all = append(all, tr.middlewares...)
				all = append(all, handlers...)
				return all, params, true
			}
			break
		}
		return nil, nil, false
	}
	return nil, nil, false
}
