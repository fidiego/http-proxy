// Package tui provides the interactive terminal UI for http-proxy.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fidiego/http-proxy/pkg/filter"
	"github.com/fidiego/http-proxy/pkg/proxy"
)

// viewMode controls which pane is shown.
type viewMode int

const (
	viewList   viewMode = iota // flow list
	viewDetail                 // request/response detail
)

// flowEventMsg wraps a proxy.FlowEvent for the Bubbletea message bus.
type flowEventMsg proxy.FlowEvent

// App is the root Bubbletea model.
type App struct {
	engine  *proxy.Engine
	store   *proxy.FlowStore
	eventCh chan proxy.FlowEvent

	// Flow state
	allFlows     []*proxy.Flow
	filtered     []*proxy.Flow
	filterExpr   string
	filterParsed filter.Filter

	// View state
	mode     viewMode
	selected int // index in filtered

	// Sub-models
	table       table.Model
	detail      viewport.Model
	filterInput textinput.Model
	filterMode  bool // is the filter input active?

	// Layout
	width  int
	height int

	// Notification bar
	notice    string
	noticeExp time.Time

	webPort int
}

// New creates a new App, subscribing to the given engine's flow store.
func New(engine *proxy.Engine, webPort int) *App {
	eventCh := engine.Store().Subscribe()

	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Method", Width: 8},
		{Title: "Status", Width: 7},
		{Title: "Upstream", Width: 12},
		{Title: "Path", Width: 45},
		{Title: "Time", Width: 7},
		{Title: "Size", Width: 7},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(colorCyan),
		Selected: tableSelectedStyle,
		Cell:     lipgloss.NewStyle(),
	})

	fi := textinput.New()
	fi.Placeholder = "filter expression (e.g. ~m POST & ~p /api)"
	fi.CharLimit = 256

	vp := viewport.New(80, 30)

	return &App{
		engine:       engine,
		store:        engine.Store(),
		eventCh:      eventCh,
		filterParsed: filter.MatchAll,
		table:        t,
		detail:       vp,
		filterInput:  fi,
		webPort:      webPort,
	}
}

// Init satisfies tea.Model.
func (a *App) Init() tea.Cmd {
	return waitForFlowEvent(a.eventCh)
}

// waitForFlowEvent returns a command that blocks until the next flow event.
func waitForFlowEvent(ch chan proxy.FlowEvent) tea.Cmd {
	return func() tea.Msg {
		return flowEventMsg(<-ch)
	}
}

// Update satisfies tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resize()

	case flowEventMsg:
		a.applyEvent(proxy.FlowEvent(msg))
		cmds = append(cmds, waitForFlowEvent(a.eventCh))

	case tea.KeyMsg:
		if a.filterMode {
			return a.updateFilterInput(msg, cmds)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "enter":
			if a.mode == viewList && len(a.filtered) > 0 {
				a.mode = viewDetail
				a.renderDetail()
			}
		case "esc", "backspace":
			if a.mode == viewDetail {
				a.mode = viewList
			}
		case "f":
			a.filterMode = true
			a.filterInput.Focus()
			return a, textinput.Blink
		case "r":
			a.replaySelected()
		case "c":
			a.copyAsCURL()
		case "d":
			a.store.Clear()
			a.allFlows = nil
			a.filtered = nil
			a.selected = 0
			a.rebuildTable()
			a.notify("Cleared all flows")
		case "up", "k":
			if a.mode == viewList {
				a.table, _ = a.table.Update(msg)
			} else {
				a.detail, _ = a.detail.Update(msg)
			}
		case "down", "j":
			if a.mode == viewList {
				a.table, _ = a.table.Update(msg)
			} else {
				a.detail, _ = a.detail.Update(msg)
			}
		case "pgup", "pgdown":
			if a.mode == viewDetail {
				a.detail, _ = a.detail.Update(msg)
			} else {
				a.table, _ = a.table.Update(msg)
			}
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) updateFilterInput(msg tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		expr := a.filterInput.Value()
		f, err := filter.Parse(expr)
		if err != nil {
			a.notify(fmt.Sprintf("invalid filter: %v", err))
		} else {
			a.filterExpr = expr
			a.filterParsed = f
			a.applyFilter()
			a.notify(fmt.Sprintf("filter: %s", expr))
		}
		a.filterMode = false
		a.filterInput.Blur()
	case "esc":
		a.filterMode = false
		a.filterInput.Blur()
	default:
		var cmd tea.Cmd
		a.filterInput, cmd = a.filterInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// View satisfies tea.Model.
func (a *App) View() string {
	if a.width == 0 {
		return "Loading…"
	}

	var b strings.Builder

	// Title bar
	upstreams := a.upstreamNames()
	title := styleStatusBar.Width(a.width).Render(
		fmt.Sprintf(" http-proxy  %s  %d flows  web: http://localhost:%d",
			upstreams, a.store.Count(), a.webPort),
	)
	b.WriteString(title)
	b.WriteString("\n")

	contentHeight := a.height - 4 // title + help + optional filter bar

	switch a.mode {
	case viewList:
		b.WriteString(a.viewList(contentHeight))
	case viewDetail:
		b.WriteString(a.viewDetailPane(contentHeight))
	}

	// Filter bar
	if a.filterMode {
		b.WriteString("\n")
		b.WriteString(styleDivider.Render(strings.Repeat("─", a.width)))
		b.WriteString("\n")
		b.WriteString(styleHelp.Render(" Filter: ") + a.filterInput.View())
	}

	// Notice / help bar
	b.WriteString("\n")
	if a.notice != "" && time.Now().Before(a.noticeExp) {
		b.WriteString(styleHelp.Width(a.width).Render(" " + a.notice))
	} else {
		if a.mode == viewList {
			b.WriteString(styleHelp.Width(a.width).Render(
				" [f]ilter [r]eplay [c]url [d]clear [q]uit  ↑↓ navigate  ⏎ detail",
			))
		} else {
			b.WriteString(styleHelp.Width(a.width).Render(
				" [esc] back  [r]eplay  [c]url  ↑↓/PgUp/PgDn scroll",
			))
		}
	}

	return b.String()
}

func (a *App) viewList(h int) string {
	a.table.SetHeight(h)
	return a.table.View()
}

func (a *App) viewDetailPane(h int) string {
	a.detail.Height = h
	return a.detail.View()
}

// applyEvent updates the in-memory flow list and rebuilds the table.
func (a *App) applyEvent(evt proxy.FlowEvent) {
	switch evt.Type {
	case proxy.FlowEventNew:
		a.allFlows = append(a.allFlows, evt.Flow)
		if a.filterParsed(evt.Flow) {
			a.filtered = append(a.filtered, evt.Flow)
		}
		a.rebuildTable()
	case proxy.FlowEventComplete, proxy.FlowEventUpdate, proxy.FlowEventError:
		// Flow was already added; refresh the table row.
		a.rebuildTable()
		if a.mode == viewDetail {
			a.renderDetail()
		}
	}
}

// applyFilter re-evaluates the filter against all known flows.
func (a *App) applyFilter() {
	a.filtered = a.filtered[:0]
	for _, f := range a.allFlows {
		if a.filterParsed(f) {
			a.filtered = append(a.filtered, f)
		}
	}
	a.rebuildTable()
}

// rebuildTable refreshes the table rows from the filtered flow slice.
func (a *App) rebuildTable() {
	rows := make([]table.Row, 0, len(a.filtered))
	for i, f := range a.filtered {
		n := fmt.Sprintf("%d", i+1)
		method := f.Request.Method
		status := "-"
		size := "-"
		if f.Response != nil {
			status = fmt.Sprintf("%d", f.Response.StatusCode)
			size = formatSize(len(f.Response.Body))
		} else if f.State == proxy.FlowStateError {
			status = "ERR"
		}
		dur := formatDur(f.Duration())
		path := f.Request.Path
		if p := f.Request.URL; p != "" && len(p) > len(path) {
			// include query string if it fits
		}
		rows = append(rows, table.Row{n, method, status, f.Upstream, path, dur, size})
	}
	a.table.SetRows(rows)
}

// renderDetail fills the viewport with request/response detail for the selected flow.
func (a *App) renderDetail() {
	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.filtered) {
		a.detail.SetContent("(no flow selected)")
		return
	}
	f := a.filtered[cursor]
	a.detail.SetContent(renderFlowDetail(f, a.width))
}

// replaySelected replays the currently selected flow.
func (a *App) replaySelected() {
	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.filtered) {
		a.notify("no flow selected")
		return
	}
	f := a.filtered[cursor]
	go func() {
		if _, err := a.engine.Replay(f.ID); err != nil {
			// The notice will appear on the next render cycle.
			_ = err
		}
	}()
	a.notify(fmt.Sprintf("replaying %s %s", f.Request.Method, f.Request.Path))
}

// copyAsCURL copies the selected flow as a cURL command.
// (Writes to the notice bar; actual clipboard integration is OS-specific.)
func (a *App) copyAsCURL() {
	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.filtered) {
		a.notify("no flow selected")
		return
	}
	f := a.filtered[cursor]
	a.notify(toCURL(f))
}

// notify sets a brief status notice.
func (a *App) notify(msg string) {
	a.notice = msg
	a.noticeExp = time.Now().Add(3 * time.Second)
}

// resize adjusts sub-model dimensions to match the terminal.
func (a *App) resize() {
	cols := a.table.Columns()
	// Give extra width to the path column.
	extra := a.width - 5 - 8 - 7 - 12 - 7 - 7 - 10 // approx fixed cols
	if extra > 20 {
		cols[4].Width = extra
	}
	a.table.SetColumns(cols)
	a.table.SetHeight(a.height - 4)
	a.detail.Width = a.width
	a.detail.Height = a.height - 4
	a.filterInput.Width = a.width - 12
}

// upstreamNames returns a compact upstream list for the title bar.
func (a *App) upstreamNames() string {
	upstreams := a.engine.Router().Upstreams()
	names := make([]string, len(upstreams))
	for i, u := range upstreams {
		names[i] = u.Name
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// Run starts the Bubbletea program, blocking until the user quits.
func Run(ctx context.Context, engine *proxy.Engine, webPort int) error {
	app := New(engine, webPort)
	p := tea.NewProgram(app, tea.WithAltScreen())

	// Stop the program when context is cancelled.
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	_, err := p.Run()
	engine.Store().Unsubscribe(app.eventCh)
	return err
}

// --- helpers ---

func renderFlowDetail(f *proxy.Flow, width int) string {
	var b strings.Builder
	half := (width - 3) / 2

	// Header
	statusStr := "-"
	if f.Response != nil {
		col := statusColor(f.Response.StatusCode)
		statusStr = lipgloss.NewStyle().Foreground(col).Bold(true).
			Render(fmt.Sprintf("%d", f.Response.StatusCode))
	} else if f.State == proxy.FlowStateError {
		statusStr = styleError.Render("ERR")
	}

	title := fmt.Sprintf("%s %s  →  %s  [%s]  %s",
		styleKeyword.Render(f.Request.Method),
		f.Request.Path,
		f.Upstream,
		formatDur(f.Duration()),
		statusStr,
	)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Tags
	if len(f.Tags) > 0 {
		for _, t := range f.Tags {
			b.WriteString(styleTag.Render(t) + " ")
		}
		b.WriteString("\n\n")
	}

	// Two-column layout: request | response
	reqCol := renderRequest(f, half)
	respCol := renderResponse(f, half)

	sep := styleDivider.Render("│")
	reqLines := strings.Split(reqCol, "\n")
	respLines := strings.Split(respCol, "\n")
	maxLines := len(reqLines)
	if len(respLines) > maxLines {
		maxLines = len(respLines)
	}

	colStyle := lipgloss.NewStyle().Width(half)
	for i := 0; i < maxLines; i++ {
		rl := ""
		if i < len(reqLines) {
			rl = reqLines[i]
		}
		sl := ""
		if i < len(respLines) {
			sl = respLines[i]
		}
		b.WriteString(colStyle.Render(rl))
		b.WriteString(sep)
		b.WriteString(colStyle.Render(sl))
		b.WriteString("\n")
	}

	return b.String()
}

func renderRequest(f *proxy.Flow, width int) string {
	if f.Request == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(styleSectionTitle.Width(width).Render("Request"))
	b.WriteString("\n")
	b.WriteString(styleKeyword.Render(f.Request.Method) + " " + f.Request.URL)
	b.WriteString("\n")
	for k, vv := range f.Request.Headers {
		for _, v := range vv {
			b.WriteString(styleGray(k+": ") + truncateStr(v, width-len(k)-4))
			b.WriteString("\n")
		}
	}
	if len(f.Request.Body) > 0 {
		b.WriteString("\n")
		body := prettyBody(f.Request.Headers.Get("Content-Type"), f.Request.Body)
		b.WriteString(body)
		if f.Request.BodyTruncated {
			b.WriteString(styleError.Render("\n… (truncated)"))
		}
	}
	return b.String()
}

func renderResponse(f *proxy.Flow, width int) string {
	if f.Response == nil {
		if f.Error != "" {
			return styleSectionTitle.Width(width).Render("Response") + "\n" +
				styleError.Render("Error: "+f.Error)
		}
		return styleSectionTitle.Width(width).Render("Response") + "\n(pending)"
	}
	var b strings.Builder
	col := statusColor(f.Response.StatusCode)
	b.WriteString(styleSectionTitle.Width(width).Render("Response"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(col).Bold(true).
		Render(fmt.Sprintf("%d", f.Response.StatusCode)))
	b.WriteString("\n")
	for k, vv := range f.Response.Headers {
		for _, v := range vv {
			b.WriteString(styleGray(k+": ") + truncateStr(v, width-len(k)-4))
			b.WriteString("\n")
		}
	}
	if len(f.Response.Body) > 0 {
		b.WriteString("\n")
		body := prettyBody(f.Response.Headers.Get("Content-Type"), f.Response.Body)
		b.WriteString(body)
		if f.Response.BodyTruncated {
			b.WriteString(styleError.Render("\n… (truncated)"))
		}
	}
	return b.String()
}

// prettyBody formats a body based on content type.
func prettyBody(contentType string, body []byte) string {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "json") {
		var v interface{}
		if err := json.Unmarshal(body, &v); err == nil {
			pretty, err := json.MarshalIndent(v, "", "  ")
			if err == nil {
				return string(pretty)
			}
		}
	}
	// Fallback: return as string, truncated.
	s := string(body)
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	return s
}

func styleGray(s string) string {
	return lipgloss.NewStyle().Foreground(colorGray).Render(s)
}

func truncateStr(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

func formatSize(n int) string {
	switch {
	case n == 0:
		return "0"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	}
}

// toCURL renders a flow as a curl command string.
func toCURL(f *proxy.Flow) string {
	if f.Request == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("curl -X %s '%s'", f.Request.Method, f.Request.URL))
	for k, vv := range f.Request.Headers {
		// Skip hop-by-hop headers.
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "transfer-encoding" {
			continue
		}
		for _, v := range vv {
			b.WriteString(fmt.Sprintf(" \\\n  -H '%s: %s'", k, v))
		}
	}
	if len(f.Request.Body) > 0 {
		body := strings.ReplaceAll(string(f.Request.Body), "'", "'\\''")
		b.WriteString(fmt.Sprintf(" \\\n  -d '%s'", body))
	}
	return b.String()
}
