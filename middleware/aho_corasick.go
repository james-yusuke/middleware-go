package middleware

import (
	"strings"
)

type acState struct {
	id         int
	depth      int
	segment    string
	isParam    bool
	paramName  string
	children   map[string]*acState
	paramChild *acState
	fail       *acState
	output     map[int][]HandlerFunc
}

type AhoCorasickRouter struct {
	root        *acState
	stateCount  int
	middlewares []HandlerFunc
	compiled    bool
}

func NewAhoCorasickRouter() *AhoCorasickRouter {
	root := &acState{
		id:       0,
		depth:    0,
		children: make(map[string]*acState),
		output:   make(map[int][]HandlerFunc),
	}
	return &AhoCorasickRouter{
		root: root,
	}
}

func (acr *AhoCorasickRouter) Use(m HandlerFunc) {
	acr.middlewares = append(acr.middlewares, m)
}

func (acr *AhoCorasickRouter) AddRoute(methodBit int, pattern string, handlers []HandlerFunc) error {
	acr.compiled = false

	p := strings.Trim(pattern, "/")
	var segs []string
	if p == "" {
		segs = []string{}
	} else {
		segs = strings.Split(p, "/")
	}

	current := acr.root

	for _, seg := range segs {
		if strings.HasPrefix(seg, ":") {
			if current.paramChild == nil {
				acr.stateCount++
				current.paramChild = &acState{
					id:        acr.stateCount,
					depth:     current.depth + 1,
					segment:   seg,
					isParam:   true,
					paramName: seg[1:],
					children:  make(map[string]*acState),
					output:    make(map[int][]HandlerFunc),
				}
			}
			current = current.paramChild
			continue
		}

		child, exists := current.children[seg]
		if !exists {
			acr.stateCount++
			child = &acState{
				id:       acr.stateCount,
				depth:    current.depth + 1,
				segment:  seg,
				children: make(map[string]*acState),
				output:   make(map[int][]HandlerFunc),
			}
			current.children[seg] = child
		}
		current = child
	}

	if current.output == nil {
		current.output = make(map[int][]HandlerFunc)
	}
	current.output[methodBit] = handlers

	return nil
}

func (acr *AhoCorasickRouter) compile() {
	if acr.compiled {
		return
	}

	queue := make([]*acState, 0)

	for _, child := range acr.root.children {
		child.fail = acr.root
		queue = append(queue, child)
	}
	if acr.root.paramChild != nil {
		acr.root.paramChild.fail = acr.root
		queue = append(queue, acr.root.paramChild)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for key, child := range current.children {
			child.fail = acr.findFailState(current.fail, key, false)
			queue = append(queue, child)
		}

		if current.paramChild != nil {
			current.paramChild.fail = acr.findFailState(current.fail, "", true)
			queue = append(queue, current.paramChild)
		}
	}

	acr.compiled = true
}

func (acr *AhoCorasickRouter) findFailState(state *acState, seg string, isParam bool) *acState {
	for state != nil {
		if isParam {
			if state.paramChild != nil {
				return state.paramChild
			}
		} else {
			if child, exists := state.children[seg]; exists {
				return child
			}
		}
		state = state.fail
	}
	return acr.root
}

func (acr *AhoCorasickRouter) Find(methodBit int, path string) ([]HandlerFunc, map[string]string, bool) {
	acr.compile()

	p := strings.Trim(path, "/")
	var segs []string
	if p == "" {
		segs = []string{}
	} else {
		segs = strings.Split(p, "/")
	}

	params := make(map[string]string)
	current := acr.root

	for i, seg := range segs {
		nextState := acr.transition(current, seg, params)

		if nextState == nil {
			nextState = acr.transitionViaFail(current, seg, params)
		}

		if nextState == nil {
			return nil, nil, false
		}

		current = nextState
		_ = i
	}

	if handlers, exists := current.output[methodBit]; exists && len(handlers) > 0 {
		all := make([]HandlerFunc, 0, len(acr.middlewares)+len(handlers))
		all = append(all, acr.middlewares...)
		all = append(all, handlers...)
		return all, params, true
	}

	return nil, nil, false
}

func (acr *AhoCorasickRouter) transition(state *acState, seg string, params map[string]string) *acState {
	if child, exists := state.children[seg]; exists {
		return child
	}

	if state.paramChild != nil {
		params[state.paramChild.paramName] = seg
		return state.paramChild
	}

	return nil
}

func (acr *AhoCorasickRouter) transitionViaFail(state *acState, seg string, params map[string]string) *acState {
	fail := state.fail
	for fail != nil && fail != acr.root {
		if child, exists := fail.children[seg]; exists {
			return child
		}
		if fail.paramChild != nil {
			params[fail.paramChild.paramName] = seg
			return fail.paramChild
		}
		fail = fail.fail
	}
	return nil
}
