// Package addons provides built-in proxy addons.
package addons

import "github.com/fidiego/http-proxy/pkg/proxy"

// CaptureAddon is a no-op addon that exists as a hook point for future
// per-flow storage extensions. Actual body capture is handled by the engine
// before the addon pipeline fires.
//
// Use this as a base for addons that need to react to every completed flow.
type CaptureAddon struct {
	OnFlowComplete func(flow *proxy.Flow)
}

func (c *CaptureAddon) OnComplete(flow *proxy.Flow) {
	if c.OnFlowComplete != nil {
		c.OnFlowComplete(flow)
	}
}
