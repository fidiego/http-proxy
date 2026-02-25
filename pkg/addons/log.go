package addons

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fidiego/http-proxy/pkg/proxy"
)

// colorFor returns an ANSI colour escape for an HTTP status code.
func colorFor(code int) string {
	switch {
	case code >= 500:
		return "\033[31m" // red
	case code >= 400:
		return "\033[33m" // yellow
	case code >= 300:
		return "\033[36m" // cyan
	case code >= 200:
		return "\033[32m" // green
	default:
		return "\033[0m"
	}
}

const resetColor = "\033[0m"

// LogAddon writes one-line summaries of completed flows to an io.Writer.
// Format mirrors mitmdump: METHOD STATUS HOST PATH [duration] [size]
type LogAddon struct {
	w      io.Writer
	noColor bool
}

// NewLogAddon creates a LogAddon that writes to w.
func NewLogAddon(w io.Writer, noColor bool) *LogAddon {
	return &LogAddon{w: w, noColor: noColor}
}

func (l *LogAddon) OnComplete(flow *proxy.Flow) {
	l.write(flow)
}

func (l *LogAddon) OnError(flow *proxy.Flow, _ error) {
	l.write(flow)
}

func (l *LogAddon) write(flow *proxy.Flow) {
	if flow.Request == nil {
		return
	}

	method := fmt.Sprintf("%-7s", flow.Request.Method)
	host := flow.Request.Host
	if host == "" {
		host = "-"
	}

	path := flow.Request.Path
	if path == "" {
		path = "/"
	}

	dur := formatDuration(flow.Duration())

	var statusPart string
	if flow.Response != nil {
		code := flow.Response.StatusCode
		codeStr := fmt.Sprintf("%d", code)
		size := formatSize(len(flow.Response.Body))
		if !l.noColor {
			statusPart = fmt.Sprintf("%s%s%s %s", colorFor(code), codeStr, resetColor, size)
		} else {
			statusPart = fmt.Sprintf("%s %s", codeStr, size)
		}
	} else {
		if !l.noColor {
			statusPart = "\033[31mERR\033[0m"
		} else {
			statusPart = "ERR"
		}
	}

	tags := ""
	if len(flow.Tags) > 0 {
		tags = " [" + strings.Join(flow.Tags, ",") + "]"
	}

	fmt.Fprintf(l.w, "%s %s  %-25s %-50s %s %s%s\n",
		method, statusPart, truncate(host, 25), truncate(path, 50), dur, flow.Upstream, tags)
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%3dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%3dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

func formatSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
