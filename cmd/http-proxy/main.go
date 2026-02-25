package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/fidiego/http-proxy/pkg/addons"
	"github.com/fidiego/http-proxy/pkg/config"
	"github.com/fidiego/http-proxy/pkg/proxy"
	"github.com/fidiego/http-proxy/pkg/tui"
	"github.com/fidiego/http-proxy/pkg/web"
)

var rootCmd = &cobra.Command{
	Use:   "http-proxy",
	Short: "Interactive HTTP reverse proxy for local development",
	Long: `http-proxy is a reverse proxy that captures, inspects, and replays
HTTP traffic across local development services.

Config file (proxy.yml) is loaded automatically from the current directory.
CLI flags override config file values.

Examples:
  # Single upstream
  http-proxy --upstream http://localhost:8081

  # Multiple upstreams with path routing
  http-proxy --route /api=http://localhost:8081 --route /runner=http://localhost:8083

  # Use a config file
  http-proxy --config proxy.yml

  # Print an example config file
  http-proxy init`,
	RunE: run,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Print an example proxy.yml to stdout",
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Print(config.Example())
		return nil
	},
}

var (
	flagConfig   string
	flagListen   string
	flagUpstream string
	flagRoutes   []string
	flagWebPort  int
	flagMaxFlows int
	flagNoTUI    bool
	flagNoColor  bool
)

func init() {
	rootCmd.Flags().StringVar(&flagConfig, "config", "",
		"path to config file (default: proxy.yml in current directory)")
	rootCmd.Flags().StringVar(&flagListen, "listen", "",
		"proxy listen address (default: :9090)")
	rootCmd.Flags().StringVar(&flagUpstream, "upstream", "",
		"single upstream target URL (e.g. http://localhost:8081)")
	rootCmd.Flags().StringArrayVar(&flagRoutes, "route", nil,
		"path-routed upstream in PREFIX=TARGET form (e.g. /api=http://localhost:8081); repeatable")
	rootCmd.Flags().IntVar(&flagWebPort, "web-port", 0,
		"port for web inspection UI (default: 9091; set to 0 to disable)")
	rootCmd.Flags().IntVar(&flagMaxFlows, "max-flows", 0,
		"maximum number of flows to keep in memory (default: 1000)")
	rootCmd.Flags().BoolVar(&flagNoTUI, "no-tui", false,
		"disable the interactive terminal UI (log to stdout only)")
	rootCmd.Flags().BoolVar(&flagNoColor, "no-color", false,
		"disable ANSI colours in log output")

	rootCmd.AddCommand(initCmd)
}

func run(cmd *cobra.Command, _ []string) error {
	// 1. Start from an empty options struct; proxy.New will apply defaults.
	opts := proxy.Options{}

	// 2. Load config file.
	cfgPath := flagConfig
	if cfgPath == "" {
		cfgPath = config.FindDefault(".")
	}
	noTUI := false
	noColor := false
	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "loaded config: %s\n", cfgPath)
		opts = cfg.ToOptions()
		noTUI = cfg.NoTUI
		noColor = cfg.NoColor
	}

	// 3. CLI flags override config file values (only when explicitly set).
	f := cmd.Flags()
	if f.Changed("listen") {
		opts.ListenAddr = flagListen
	}
	if f.Changed("web-port") {
		opts.WebPort = flagWebPort
	}
	if f.Changed("max-flows") {
		opts.MaxFlows = flagMaxFlows
	}
	if f.Changed("no-tui") {
		noTUI = flagNoTUI
	}
	if f.Changed("no-color") {
		noColor = flagNoColor
	}

	// --upstream and --route replace (not merge with) the config file's upstreams
	// when either flag is explicitly provided.
	if f.Changed("upstream") || f.Changed("route") {
		cliUpstreams, err := buildUpstreams()
		if err != nil {
			return err
		}
		opts.Upstreams = cliUpstreams
	}

	if len(opts.Upstreams) == 0 {
		return fmt.Errorf("at least one upstream is required (use --upstream, --route, or a config file)")
	}

	engine, err := proxy.New(opts)
	if err != nil {
		return fmt.Errorf("create engine: %w", err)
	}

	engine.Addons().Add(addons.NewLogAddon(os.Stdout, noTUI || noColor))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		fmt.Fprintf(os.Stderr, "proxy listening on %s\n", engine.Options().ListenAddr)
		return engine.Start(ctx)
	})

	if engine.Options().WebPort > 0 {
		webSrv := web.New(engine, engine.Options().WebPort)
		g.Go(func() error {
			return webSrv.Start(ctx)
		})
	}

	if !noTUI && isTerminal() {
		g.Go(func() error {
			return tui.Run(ctx, engine, engine.Options().WebPort)
		})
	}

	return g.Wait()
}

// buildUpstreams constructs the upstream list from --upstream / --route flags.
func buildUpstreams() ([]proxy.Upstream, error) {
	var upstreams []proxy.Upstream

	if flagUpstream != "" {
		upstreams = append(upstreams, proxy.Upstream{
			Name:   "default",
			Prefix: "/",
			Target: flagUpstream,
		})
	}

	for _, r := range flagRoutes {
		parts := strings.SplitN(r, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --route %q: expected PREFIX=TARGET", r)
		}
		prefix, target := parts[0], parts[1]
		name := strings.TrimPrefix(prefix, "/")
		if name == "" {
			name = "default"
		}
		upstreams = append(upstreams, proxy.Upstream{
			Name:   name,
			Prefix: prefix,
			Target: target,
		})
	}

	return upstreams, nil
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
