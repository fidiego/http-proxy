package proxy

import (
	"net/http"
	"sync"
	"time"
)

// FlowState describes the current lifecycle stage of a Flow.
type FlowState string

const (
	FlowStateActive      FlowState = "active"
	FlowStateIntercepted FlowState = "intercepted"
	FlowStateComplete    FlowState = "complete"
	FlowStateError       FlowState = "error"
)

// CapturedRequest holds a snapshot of an HTTP request.
type CapturedRequest struct {
	Method        string      `json:"method"`
	URL           string      `json:"url"`
	Path          string      `json:"path"`
	Host          string      `json:"host"`
	Headers       http.Header `json:"headers"`
	Body          []byte      `json:"body,omitempty"`
	Proto         string      `json:"proto"`
	BodyTruncated bool        `json:"bodyTruncated,omitempty"`
}

// CapturedResponse holds a snapshot of an HTTP response.
type CapturedResponse struct {
	StatusCode    int         `json:"statusCode"`
	Headers       http.Header `json:"headers"`
	Body          []byte      `json:"body,omitempty"`
	Proto         string      `json:"proto"`
	BodyTruncated bool        `json:"bodyTruncated,omitempty"`
}

// Flow represents a complete HTTP transaction.
type Flow struct {
	ID       string `json:"id"`
	Upstream string `json:"upstream"` // name of the upstream that handled this

	Request  *CapturedRequest  `json:"request"`
	Response *CapturedResponse `json:"response,omitempty"`
	Error    string            `json:"error,omitempty"`

	State FlowState `json:"state"`
	Tags  []string  `json:"tags,omitempty"`

	Timestamps struct {
		Created       time.Time `json:"created"`
		RequestDone   time.Time `json:"requestDone"`
		ResponseStart time.Time `json:"responseStart,omitempty"`
		ResponseDone  time.Time `json:"responseDone,omitempty"`
	} `json:"timestamps"`

	// mu protects resumeCh and killed, used for intercept/resume.
	mu       sync.Mutex
	resumeCh chan struct{}
	killed   bool
}

// Duration returns elapsed time from flow creation to response completion,
// or to now if the flow is still in-flight.
func (f *Flow) Duration() time.Duration {
	if !f.Timestamps.ResponseDone.IsZero() {
		return f.Timestamps.ResponseDone.Sub(f.Timestamps.Created)
	}
	return time.Since(f.Timestamps.Created)
}

// Intercept pauses the flow until Resume or Kill is called.
func (f *Flow) Intercept() {
	f.mu.Lock()
	f.State = FlowStateIntercepted
	f.resumeCh = make(chan struct{})
	f.mu.Unlock()
	<-f.resumeCh
}

// Resume continues a paused (intercepted) flow.
func (f *Flow) Resume() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resumeCh != nil && !f.killed {
		close(f.resumeCh)
		f.resumeCh = nil
	}
	f.State = FlowStateActive
}

// Kill terminates a flow; if it is intercepted it will be unblocked.
func (f *Flow) Kill() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killed = true
	if f.resumeCh != nil {
		close(f.resumeCh)
		f.resumeCh = nil
	}
	f.State = FlowStateError
	f.Error = "flow killed"
}

// FlowEventType describes the kind of change that occurred to a flow.
type FlowEventType string

const (
	FlowEventNew      FlowEventType = "new"
	FlowEventUpdate   FlowEventType = "update"
	FlowEventComplete FlowEventType = "complete"
	FlowEventError    FlowEventType = "error"
)

// FlowEvent carries a flow change notification to subscribers.
type FlowEvent struct {
	Type FlowEventType `json:"type"`
	Flow *Flow         `json:"flow"`
}
