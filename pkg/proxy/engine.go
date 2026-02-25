package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type contextKey string

const flowContextKey contextKey = "flow"

// Engine is the core proxy. It routes requests to upstreams, captures flows,
// and dispatches them through the addon pipeline.
type Engine struct {
	store   *FlowStore
	addons  *AddonManager
	router  *Router
	proxies map[string]*httputil.ReverseProxy
	opts    Options
	server  *http.Server
	webSrv  *http.Server
}

// New creates a new Engine with the given options.
func New(opts Options) (*Engine, error) {
	opts.setDefaults()

	router, err := NewRouter(opts.Upstreams)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		store:   NewFlowStore(opts.MaxFlows),
		addons:  NewAddonManager(),
		router:  router,
		proxies: make(map[string]*httputil.ReverseProxy),
		opts:    opts,
	}

	for i := range router.upstreams {
		u := &router.upstreams[i]
		p := &httputil.ReverseProxy{
			Director:       Director(u),
			ModifyResponse: e.modifyResponse,
			ErrorHandler:   e.errorHandler,
			FlushInterval:  -1, // flush immediately for streaming support
		}
		e.proxies[u.Name] = p
	}

	return e, nil
}

// Options returns the resolved options the engine was started with.
func (e *Engine) Options() Options { return e.opts }

// Store returns the flow store (read-only access for UI components).
func (e *Engine) Store() *FlowStore { return e.store }

// Addons returns the addon manager so callers can register addons.
func (e *Engine) Addons() *AddonManager { return e.addons }

// Router returns the router (for UI display of configured upstreams).
func (e *Engine) Router() *Router { return e.router }

// Start runs the proxy and (optionally) the web UI server until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	e.server = &http.Server{
		Addr:    e.opts.ListenAddr,
		Handler: e,
	}

	g.Go(func() error {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("proxy server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.server.Shutdown(shutCtx)
		return nil
	})

	return g.Wait()
}

// ServeHTTP implements http.Handler. It is the main proxy entry point.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upstream := e.router.Match(r)
	if upstream == nil {
		http.Error(w, "no upstream matched", http.StatusBadGateway)
		return
	}

	flow := e.newFlow(r, upstream)
	e.store.Add(flow)

	if err := captureRequestBody(flow, r, e.opts.MaxBodySize); err != nil {
		flow.State = FlowStateError
		flow.Error = fmt.Sprintf("capture request: %v", err)
		e.store.Update(flow, FlowEventError)
		http.Error(w, "internal proxy error", http.StatusInternalServerError)
		return
	}

	flow.Timestamps.RequestDone = time.Now()

	e.addons.FireRequest(flow)

	if flow.killed {
		http.Error(w, "flow killed", http.StatusBadGateway)
		return
	}

	// Attach the flow to the request context so modifyResponse can find it.
	r = r.WithContext(context.WithValue(r.Context(), flowContextKey, flow))

	proxy, ok := e.proxies[upstream.Name]
	if !ok {
		http.Error(w, "upstream not configured", http.StatusBadGateway)
		return
	}
	proxy.ServeHTTP(w, r)
}

// modifyResponse is called by the reverse proxy with the upstream response.
func (e *Engine) modifyResponse(resp *http.Response) error {
	flow, ok := resp.Request.Context().Value(flowContextKey).(*Flow)
	if !ok {
		return nil
	}

	flow.Timestamps.ResponseStart = time.Now()

	if err := captureResponseBody(flow, resp, e.opts.MaxBodySize); err != nil {
		// Don't fail the proxy; just mark the body capture as failed.
		flow.Response.Body = nil
		flow.Response.BodyTruncated = true
	}

	flow.Timestamps.ResponseDone = time.Now()
	flow.State = FlowStateComplete

	e.addons.FireResponse(flow)
	e.addons.FireComplete(flow)
	e.store.Update(flow, FlowEventComplete)

	return nil
}

// errorHandler is called by the reverse proxy when the upstream is unreachable.
func (e *Engine) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	flow, ok := r.Context().Value(flowContextKey).(*Flow)
	if ok {
		flow.State = FlowStateError
		flow.Error = err.Error()
		flow.Timestamps.ResponseDone = time.Now()
		e.addons.FireError(flow, err)
		e.store.Update(flow, FlowEventError)
	}
	http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
}

// newFlow builds a Flow skeleton from the incoming request.
func (e *Engine) newFlow(r *http.Request, upstream *Upstream) *Flow {
	f := &Flow{
		ID:       uuid.New().String(),
		Upstream: upstream.Name,
		State:    FlowStateActive,
	}
	f.Timestamps.Created = time.Now()
	f.Request = &CapturedRequest{
		Method:  r.Method,
		URL:     r.URL.String(),
		Path:    r.URL.Path,
		Host:    r.Host,
		Headers: r.Header.Clone(),
		Proto:   r.Proto,
	}
	return f
}

// Replay re-sends the request from a captured flow through the proxy engine.
// The replayed flow is stored as a new entry and returned.
func (e *Engine) Replay(flowID string) (*Flow, error) {
	original := e.store.Get(flowID)
	if original == nil {
		return nil, fmt.Errorf("flow %q not found", flowID)
	}
	if original.Request == nil {
		return nil, fmt.Errorf("flow %q has no captured request", flowID)
	}

	req, err := rebuildRequest(original.Request)
	if err != nil {
		return nil, fmt.Errorf("rebuild request: %w", err)
	}

	upstream := e.router.Match(req)
	if upstream == nil {
		return nil, fmt.Errorf("no upstream for path %q", req.URL.Path)
	}

	flow := e.newFlow(req, upstream)
	flow.Tags = append(flow.Tags, "replay", "replay:"+flowID)
	flow.Request = cloneRequest(original.Request)
	e.store.Add(flow)

	// Forward via the upstream proxy, capturing response into a recorder.
	rec := &responseRecorder{header: make(http.Header), code: 200}
	req = req.WithContext(context.WithValue(req.Context(), flowContextKey, flow))
	proxy, ok := e.proxies[upstream.Name]
	if !ok {
		return nil, fmt.Errorf("upstream %q not configured", upstream.Name)
	}
	proxy.ServeHTTP(rec, req)

	return e.store.Get(flow.ID), nil
}

// captureRequestBody reads up to maxBytes of the request body and stores it on the flow.
func captureRequestBody(flow *Flow, r *http.Request, maxBytes int64) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	body, truncated, err := readLimited(r.Body, maxBytes)
	if err != nil {
		return err
	}
	// Replace r.Body so the reverse proxy can still read it.
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	flow.Request.Body = body
	flow.Request.BodyTruncated = truncated
	return nil
}

// captureResponseBody reads up to maxBytes of the response body and stores it on the flow.
func captureResponseBody(flow *Flow, resp *http.Response, maxBytes int64) error {
	captured := &CapturedResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Proto:      resp.Proto,
	}
	flow.Response = captured

	if resp.Body == nil {
		return nil
	}

	body, truncated, err := readLimited(resp.Body, maxBytes)
	if err != nil {
		return err
	}
	// Replace resp.Body so the reverse proxy can still send it.
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))

	captured.Body = body
	captured.BodyTruncated = truncated
	return nil
}

// readLimited reads at most maxBytes from r, then closes r.
// Returns the bytes read and whether the source had more data (truncated).
func readLimited(r io.ReadCloser, maxBytes int64) ([]byte, bool, error) {
	defer r.Close()
	limit := maxBytes + 1
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

// rebuildRequest constructs a new *http.Request from a CapturedRequest.
func rebuildRequest(cr *CapturedRequest) (*http.Request, error) {
	req, err := http.NewRequest(cr.Method, cr.URL, bytes.NewReader(cr.Body))
	if err != nil {
		return nil, err
	}
	for k, vv := range cr.Headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	return req, nil
}

// cloneRequest returns a copy of a CapturedRequest (with a copy of the body slice).
func cloneRequest(cr *CapturedRequest) *CapturedRequest {
	body := make([]byte, len(cr.Body))
	copy(body, cr.Body)
	return &CapturedRequest{
		Method:        cr.Method,
		URL:           cr.URL,
		Path:          cr.Path,
		Host:          cr.Host,
		Headers:       cr.Headers.Clone(),
		Body:          body,
		Proto:         cr.Proto,
		BodyTruncated: cr.BodyTruncated,
	}
}

// responseRecorder is a minimal http.ResponseWriter used for internal replay.
type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) WriteHeader(code int)        { r.code = code }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
