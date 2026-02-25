// Package filter provides a simple expression language for matching flows.
//
// Syntax:
//
//	~m METHOD   match HTTP method (substring)
//	~s CODE     match response status code (prefix, e.g. "5" matches 5xx)
//	~p PATH     match URL path (substring)
//	~h KEY:VAL  match header key containing VAL (substring)
//	~b TEXT     match request or response body (substring)
//	~u NAME     match upstream name (substring)
//	!EXPR       negate
//	A & B       AND
//	A | B       OR
//	(EXPR)      grouping
package filter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fidiego/http-proxy/pkg/proxy"
)

// Filter is a compiled predicate over a Flow.
type Filter func(flow *proxy.Flow) bool

// MatchAll matches every flow.
var MatchAll Filter = func(_ *proxy.Flow) bool { return true }

// Parse compiles a filter expression string. Returns MatchAll for empty input.
func Parse(expr string) (Filter, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return MatchAll, nil
	}
	p := &parser{input: expr, pos: 0}
	f, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected token at position %d: %q", p.pos, p.input[p.pos:])
	}
	return f, nil
}

// parser is a simple recursive-descent parser.
type parser struct {
	input string
	pos   int
}

func (p *parser) peek() byte {
	p.skipWS()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

// parseOr handles A | B.
func (p *parser) parseOr() (Filter, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '|' {
			p.pos++
			right, err := p.parseAnd()
			if err != nil {
				return nil, err
			}
			l, r := left, right
			left = func(f *proxy.Flow) bool { return l(f) || r(f) }
		} else {
			break
		}
	}
	return left, nil
}

// parseAnd handles A & B.
func (p *parser) parseAnd() (Filter, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos < len(p.input) && p.input[p.pos] == '&' {
			p.pos++
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			l, r := left, right
			left = func(f *proxy.Flow) bool { return l(f) && r(f) }
		} else {
			break
		}
	}
	return left, nil
}

// parseNot handles !EXPR.
func (p *parser) parseNot() (Filter, error) {
	p.skipWS()
	if p.pos < len(p.input) && p.input[p.pos] == '!' {
		p.pos++
		inner, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		return func(f *proxy.Flow) bool { return !inner(f) }, nil
	}
	return p.parseAtom()
}

// parseAtom handles primitives and parenthesised groups.
func (p *parser) parseAtom() (Filter, error) {
	p.skipWS()
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, fmt.Errorf("expected closing ')'")
		}
		p.pos++
		return inner, nil
	}
	return p.parsePrimitive()
}

// parsePrimitive handles ~x OPERAND tokens.
func (p *parser) parsePrimitive() (Filter, error) {
	p.skipWS()
	if p.pos+1 >= len(p.input) || p.input[p.pos] != '~' {
		return nil, fmt.Errorf("expected filter expression starting with '~' at position %d", p.pos)
	}
	p.pos++ // consume '~'
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("expected filter type after '~'")
	}
	kind := p.input[p.pos]
	p.pos++ // consume kind character
	p.skipWS()

	arg, err := p.parseArg()
	if err != nil {
		return nil, err
	}

	switch kind {
	case 'm':
		return methodFilter(arg), nil
	case 's':
		return statusFilter(arg), nil
	case 'p':
		return pathFilter(arg), nil
	case 'h':
		return headerFilter(arg), nil
	case 'b':
		return bodyFilter(arg), nil
	case 'u':
		return upstreamFilter(arg), nil
	default:
		return nil, fmt.Errorf("unknown filter type %q", string(kind))
	}
}

// parseArg reads the next whitespace-delimited token or quoted string.
func (p *parser) parseArg() (string, error) {
	p.skipWS()
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("expected argument")
	}
	if p.input[p.pos] == '"' {
		return p.parseQuoted()
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '&' || c == '|' || c == ')' {
			break
		}
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("empty argument at position %d", p.pos)
	}
	return p.input[start:p.pos], nil
}

func (p *parser) parseQuoted() (string, error) {
	p.pos++ // consume opening '"'
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '"' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("unterminated quoted string")
	}
	s := p.input[start:p.pos]
	p.pos++ // consume closing '"'
	return s, nil
}

// --- primitive filter constructors ---

func methodFilter(arg string) Filter {
	upper := strings.ToUpper(arg)
	return func(f *proxy.Flow) bool {
		if f.Request == nil {
			return false
		}
		return strings.Contains(strings.ToUpper(f.Request.Method), upper)
	}
}

func statusFilter(arg string) Filter {
	return func(f *proxy.Flow) bool {
		if f.Response == nil {
			return false
		}
		code := strconv.Itoa(f.Response.StatusCode)
		return strings.HasPrefix(code, arg)
	}
}

func pathFilter(arg string) Filter {
	lower := strings.ToLower(arg)
	return func(f *proxy.Flow) bool {
		if f.Request == nil {
			return false
		}
		return strings.Contains(strings.ToLower(f.Request.Path), lower)
	}
}

func headerFilter(arg string) Filter {
	// arg is "Key:Value" or just "Key"
	parts := strings.SplitN(arg, ":", 2)
	key := strings.ToLower(parts[0])
	val := ""
	if len(parts) == 2 {
		val = strings.ToLower(parts[1])
	}
	return func(f *proxy.Flow) bool {
		if f.Request != nil {
			for k, vv := range f.Request.Headers {
				if strings.Contains(strings.ToLower(k), key) {
					if val == "" {
						return true
					}
					for _, v := range vv {
						if strings.Contains(strings.ToLower(v), val) {
							return true
						}
					}
				}
			}
		}
		if f.Response != nil {
			for k, vv := range f.Response.Headers {
				if strings.Contains(strings.ToLower(k), key) {
					if val == "" {
						return true
					}
					for _, v := range vv {
						if strings.Contains(strings.ToLower(v), val) {
							return true
						}
					}
				}
			}
		}
		return false
	}
}

func bodyFilter(arg string) Filter {
	lower := strings.ToLower(arg)
	return func(f *proxy.Flow) bool {
		if f.Request != nil && strings.Contains(strings.ToLower(string(f.Request.Body)), lower) {
			return true
		}
		if f.Response != nil && strings.Contains(strings.ToLower(string(f.Response.Body)), lower) {
			return true
		}
		return false
	}
}

func upstreamFilter(arg string) Filter {
	lower := strings.ToLower(arg)
	return func(f *proxy.Flow) bool {
		return strings.Contains(strings.ToLower(f.Upstream), lower)
	}
}
