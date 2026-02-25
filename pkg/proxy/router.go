package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Upstream defines a single proxy target.
type Upstream struct {
	Name   string // display name (e.g. "ctl-api")
	Prefix string // URL path prefix to match (e.g. "/api"); use "/" for catch-all
	Target string // target base URL (e.g. "http://localhost:8081")
	parsed *url.URL
}

// Router routes incoming requests to upstreams based on path prefix.
// Longer prefixes take precedence over shorter ones.
type Router struct {
	upstreams []Upstream
}

// NewRouter validates and prepares the given upstreams for routing.
func NewRouter(upstreams []Upstream) (*Router, error) {
	r := &Router{}
	for _, u := range upstreams {
		if u.Prefix == "" {
			u.Prefix = "/"
		}
		parsed, err := url.Parse(u.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q for upstream %q: %w", u.Target, u.Name, err)
		}
		u.parsed = parsed
		r.upstreams = append(r.upstreams, u)
	}
	// Longest prefix wins.
	sort.Slice(r.upstreams, func(i, j int) bool {
		return len(r.upstreams[i].Prefix) > len(r.upstreams[j].Prefix)
	})
	return r, nil
}

// Match returns the best-matching upstream for the given request path, or nil.
func (r *Router) Match(req *http.Request) *Upstream {
	path := req.URL.Path
	for i := range r.upstreams {
		u := &r.upstreams[i]
		if u.Prefix == "/" || strings.HasPrefix(path, u.Prefix) {
			return u
		}
	}
	return nil
}

// Upstreams returns a read-only copy of the configured upstreams.
func (r *Router) Upstreams() []Upstream {
	cp := make([]Upstream, len(r.upstreams))
	copy(cp, r.upstreams)
	return cp
}

// Director returns an http.Request director for use with httputil.ReverseProxy.
// It rewrites the outgoing request URL to point at the upstream target.
func Director(upstream *Upstream) func(*http.Request) {
	target := upstream.parsed
	return func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// Prepend the target's base path if it has one.
		if p := target.Path; p != "" && p != "/" {
			req.URL.Path = strings.TrimSuffix(p, "/") + req.URL.Path
		}

		req.Host = target.Host

		// Propagate the real client IP.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			req.Header.Set("X-Forwarded-For", strings.Join(prior, ", ")+", "+req.RemoteAddr)
		} else {
			req.Header.Set("X-Forwarded-For", req.RemoteAddr)
		}
	}
}
