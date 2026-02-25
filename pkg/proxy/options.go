package proxy

const (
	DefaultListenAddr = ":9090"
	DefaultWebPort    = 9091
	DefaultMaxFlows   = 1000
	DefaultMaxBody    = 1 << 20 // 1 MiB
)

// Options configures the proxy engine.
type Options struct {
	// ListenAddr is the address for the proxy HTTP server (e.g. ":9090").
	ListenAddr string

	// WebPort is the port for the web inspection UI. 0 disables it.
	WebPort int

	// Upstreams defines the routing table.
	Upstreams []Upstream

	// MaxFlows is the ring-buffer capacity for the flow store.
	MaxFlows int

	// MaxBodySize is the maximum number of bytes captured per request/response body.
	MaxBodySize int64
}

func (o *Options) setDefaults() {
	if o.ListenAddr == "" {
		o.ListenAddr = DefaultListenAddr
	}
	if o.WebPort == 0 {
		o.WebPort = DefaultWebPort
	}
	if o.MaxFlows == 0 {
		o.MaxFlows = DefaultMaxFlows
	}
	if o.MaxBodySize == 0 {
		o.MaxBodySize = DefaultMaxBody
	}
}
