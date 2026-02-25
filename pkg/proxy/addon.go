package proxy

// RequestHook is called after the full request body is read, before forwarding.
type RequestHook interface {
	OnRequest(flow *Flow)
}

// ResponseHook is called after the full response body is read, before returning to the client.
type ResponseHook interface {
	OnResponse(flow *Flow)
}

// CompleteHook is called when a flow finishes successfully.
type CompleteHook interface {
	OnComplete(flow *Flow)
}

// ErrorHook is called when an error occurs during proxying.
type ErrorHook interface {
	OnError(flow *Flow, err error)
}

// Addon is a marker interface; addons implement whichever hook interfaces they need.
type Addon interface{}

// AddonManager dispatches flow lifecycle events to registered addons in order.
type AddonManager struct {
	addons []Addon
}

// NewAddonManager returns an empty AddonManager.
func NewAddonManager() *AddonManager {
	return &AddonManager{}
}

// Add registers one or more addons.
func (m *AddonManager) Add(addons ...Addon) {
	m.addons = append(m.addons, addons...)
}

// FireRequest calls OnRequest on every addon that implements RequestHook.
func (m *AddonManager) FireRequest(flow *Flow) {
	for _, a := range m.addons {
		if h, ok := a.(RequestHook); ok {
			h.OnRequest(flow)
		}
	}
}

// FireResponse calls OnResponse on every addon that implements ResponseHook.
func (m *AddonManager) FireResponse(flow *Flow) {
	for _, a := range m.addons {
		if h, ok := a.(ResponseHook); ok {
			h.OnResponse(flow)
		}
	}
}

// FireComplete calls OnComplete on every addon that implements CompleteHook.
func (m *AddonManager) FireComplete(flow *Flow) {
	for _, a := range m.addons {
		if h, ok := a.(CompleteHook); ok {
			h.OnComplete(flow)
		}
	}
}

// FireError calls OnError on every addon that implements ErrorHook.
func (m *AddonManager) FireError(flow *Flow, err error) {
	for _, a := range m.addons {
		if h, ok := a.(ErrorHook); ok {
			h.OnError(flow, err)
		}
	}
}
