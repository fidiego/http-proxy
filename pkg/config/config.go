// Package config handles loading http-proxy configuration from YAML files.
//
// Loading priority (later wins):
//
//  1. Built-in defaults
//  2. Config file (proxy.yml in cwd, or --config path)
//  3. Explicit CLI flags
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/fidiego/http-proxy/pkg/proxy"
)

// DefaultFilenames lists the config file names searched in the current
// directory when --config is not given.
var DefaultFilenames = []string{"proxy.yml", "proxy.yaml", ".proxy.yml"}

// UpstreamConfig is the YAML representation of a single upstream.
type UpstreamConfig struct {
	Name   string `yaml:"name"`
	Prefix string `yaml:"prefix"`
	Target string `yaml:"target"`
}

// Config is the full YAML configuration for http-proxy.
type Config struct {
	// Listen is the proxy server address (e.g. ":9090").
	Listen string `yaml:"listen"`

	// WebPort is the port for the web inspection UI. 0 disables it.
	WebPort *int `yaml:"web_port"`

	// NoTUI disables the interactive terminal UI.
	NoTUI bool `yaml:"no_tui"`

	// NoColor disables ANSI colours in log output.
	NoColor bool `yaml:"no_color"`

	// MaxFlows is the ring-buffer capacity for the flow store.
	MaxFlows *int `yaml:"max_flows"`

	// MaxBodySize is the max bytes captured per request/response body.
	MaxBodySize *int64 `yaml:"max_body_size"`

	// Upstream is a shorthand for a single catch-all upstream.
	// Equivalent to a single entry in Upstreams with prefix "/".
	Upstream string `yaml:"upstream"`

	// Upstreams defines the routing table for multi-upstream mode.
	Upstreams []UpstreamConfig `yaml:"upstreams"`
}

// Load reads and parses a YAML config file from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	return &cfg, nil
}

// FindDefault looks for a config file in dir using DefaultFilenames.
// Returns the path of the first file found, or "" if none exist.
func FindDefault(dir string) string {
	for _, name := range DefaultFilenames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ToOptions converts the Config into proxy.Options, applying built-in defaults
// for any fields left unset.
func (c *Config) ToOptions() proxy.Options {
	opts := proxy.Options{}

	if c.Listen != "" {
		opts.ListenAddr = c.Listen
	}
	if c.WebPort != nil {
		opts.WebPort = *c.WebPort
	}
	if c.MaxFlows != nil {
		opts.MaxFlows = *c.MaxFlows
	}
	if c.MaxBodySize != nil {
		opts.MaxBodySize = *c.MaxBodySize
	}

	// Build upstream list.
	if c.Upstream != "" {
		opts.Upstreams = append(opts.Upstreams, proxy.Upstream{
			Name:   "default",
			Prefix: "/",
			Target: c.Upstream,
		})
	}
	for _, u := range c.Upstreams {
		prefix := u.Prefix
		if prefix == "" {
			prefix = "/"
		}
		name := u.Name
		if name == "" {
			name = u.Prefix
		}
		opts.Upstreams = append(opts.Upstreams, proxy.Upstream{
			Name:   name,
			Prefix: prefix,
			Target: u.Target,
		})
	}

	return opts
}

// Example returns the canonical example config as a YAML string.
func Example() string {
	return `# http-proxy configuration
# All fields are optional; CLI flags take precedence over this file.

# Proxy listen address.
listen: ":9090"

# Port for the web inspection UI. Set to 0 to disable.
web_port: 9091

# Disable the interactive terminal UI (log to stdout instead).
no_tui: false

# Disable ANSI colors in log output.
no_color: false

# Maximum number of flows held in memory (ring buffer).
max_flows: 1000

# Maximum bytes captured per request/response body (default: 1048576 = 1 MiB).
max_body_size: 1048576

# --- Upstream routing ---

# Single upstream: proxy everything to one target.
# upstream: http://localhost:8081

# Multi-upstream: route by path prefix (longer prefixes win).
upstreams:
  - name: ctl-api
    prefix: /api
    target: http://localhost:8081
  - name: runner
    prefix: /runner
    target: http://localhost:8083
  - name: dashboard
    prefix: /
    target: http://localhost:4000
`
}
