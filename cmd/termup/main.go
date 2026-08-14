package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// defaultAddr is used when neither --addr nor TERMUP_ADDR is set.
// Local default is the unix socket (file permissions act as auth);
// remote uses an http:// TCP address.
const defaultAddr = "unix:///tmp/termupd.sock"

// pollInterval is how often the dashboard re-fetches from termupd. Kept below the
// probe interval so the view stays fresh (probe cadence is the server's).
const pollInterval = 10 * time.Second

// resolveAddr picks the server address: --addr flag > TERMUP_ADDR env > default.
func resolveAddr(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("TERMUP_ADDR"); v != "" {
		return v
	}
	return defaultAddr
}

func main() {
	addrFlag := flag.String("addr", "", "termupd server address (unix:///path or http://host:port)")
	flag.Parse()

	client, base := newClient(resolveAddr(*addrFlag))

	args := flag.Args()
	if len(args) == 0 || args[0] != "watch" {
		fmt.Fprintln(os.Stderr, "usage: termup [--addr ...] watch")
		os.Exit(2)
	}

	p := tea.NewProgram(newModel(client, base), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "termup:", err)
		os.Exit(1)
	}
}

// newClient returns an HTTP client and base URL. For unix:// it dials the
// socket; otherwise it talks plain HTTP to the given address.
func newClient(addr string) (*http.Client, string) {
	if path, ok := strings.CutPrefix(addr, "unix://"); ok {
		return &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", path)
				},
			},
		}, "http://localhost"
	}
	return &http.Client{Timeout: 5 * time.Second}, strings.TrimRight(addr, "/")
}

// ---- bubbletea model ----

type model struct {
	client    *http.Client
	base      string
	health    []api.MonitorHealthDTO
	err       error
	updated   time.Time
	width     int
	height    int
	hover     *hoverInfo
	sel       *selRef
	filter    textinput.Model
	filtering bool // true while the filter input is focused
}

// hoverInfo is the check the mouse is currently over, shown in the detail panel.
type hoverInfo struct {
	name  string
	check api.CheckDTO
}

// selRef is the keyboard selection: a monitor and a displayed bar index within
// it (0-based over the shown window). Persistent, unlike the transient hover.
type selRef struct {
	mon   int
	check int
}

type dataMsg struct {
	health []api.MonitorHealthDTO
	err    error
	at     time.Time
}

type tickMsg time.Time

func newModel(c *http.Client, base string) model {
	ti := textinput.New()
	ti.Placeholder = "name or url…"
	ti.Prompt = "" // the filter box draws its own "⌕ " label
	return model{client: c, base: base, filter: ti}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetch, tick())
}

func tick() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetch is a tea.Cmd (called, returns a msg) that polls the dashboard endpoint.
func (m model) fetch() tea.Msg {
	var dr api.DashboardResponse
	err := getJSON(m.client, m.base+"/v1/dashboard", &dr)
	return dataMsg{health: dr.Monitors, err: err, at: time.Now()}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "esc": // cancel: clear the filter and leave input mode
				m.filtering = false
				m.filter.Blur()
				m.filter.SetValue("")
				m.clampSelection()
				return m, nil
			case "enter": // apply: keep the query, leave input mode
				m.filtering = false
				m.filter.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filter, cmd = m.filter.Update(msg)
				m.clampSelection() // the filtered set may have shrunk
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "/":
			m.filtering = true
			m.hover = nil
			return m, m.filter.Focus()
		case "r":
			return m, m.fetch
		case "tab":
			m.moveMonitor(1)
		case "shift+tab":
			m.moveMonitor(-1)
		case "right", "l":
			m.moveCheck(1)
		case "left", "h":
			m.moveCheck(-1)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.MouseMsg:
		if name, c, ok := m.checkAt(msg.X, msg.Y); ok {
			m.hover = &hoverInfo{name: name, check: c}
		} else {
			m.hover = nil
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetch, tick())
	case dataMsg:
		m.err = msg.err
		if msg.err == nil {
			m.health = msg.health
			m.updated = msg.at
			m.clampSelection() // history may have shifted under the selection
		}
	}
	return m, nil
}

// visible is the monitor list after applying the filter query. All rendering,
// navigation and hit-testing operate on this, so indices track what's on screen.
func (m model) visible() []api.MonitorHealthDTO {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return m.health
	}
	out := make([]api.MonitorHealthDTO, 0, len(m.health))
	for _, h := range m.health {
		if strings.Contains(strings.ToLower(h.Name), q) || strings.Contains(strings.ToLower(h.URL), q) {
			out = append(out, h)
		}
	}
	return out
}

// ---- keyboard selection ----

func (m *model) moveMonitor(d int) {
	vis := m.visible()
	n := len(vis)
	if n == 0 {
		return
	}
	if m.sel == nil {
		m.sel = &selRef{mon: 0, check: lastCheck(vis[0])}
		return
	}
	m.sel.mon = (m.sel.mon + d%n + n) % n
	m.clampSelection()
}

func (m *model) moveCheck(d int) {
	vis := m.visible()
	if len(vis) == 0 {
		return
	}
	if m.sel == nil {
		m.sel = &selRef{mon: 0, check: lastCheck(vis[0])}
		return
	}
	m.sel.check += d
	m.clampSelection()
}

// clampSelection keeps sel within the current (filtered) data bounds.
func (m *model) clampSelection() {
	if m.sel == nil {
		return
	}
	vis := m.visible()
	if m.sel.mon >= len(vis) {
		m.sel = nil
		return
	}
	shown := shownCount(vis[m.sel.mon])
	switch {
	case shown == 0:
		m.sel.check = 0
	case m.sel.check >= shown:
		m.sel.check = shown - 1
	case m.sel.check < 0:
		m.sel.check = 0
	}
}

func (m model) View() string {
	header := titleStyle.Render("termup") + "  " +
		subtleStyle.Render("health dashboard")
	if !m.updated.IsZero() {
		header += subtleStyle.Render(" · updated " + m.updated.Format("15:04:05"))
	}

	vis := m.visible()
	var body string
	switch {
	case m.err != nil:
		body = errStyle.Render("cannot reach termupd: "+m.err.Error()) +
			subtleStyle.Render("  (is it running?)")
	case len(m.health) == 0:
		body = subtleStyle.Render("loading…")
	case len(vis) == 0:
		body = subtleStyle.Render("no monitors match the filter")
	default:
		cards := make([]string, len(vis))
		for i, h := range vis {
			sel := -1
			if m.sel != nil && m.sel.mon == i {
				sel = m.sel.check
			}
			cards[i] = renderCard(h, sel)
		}
		body = arrangeGrid(cards, m.width)
	}

	footer := helpStyle.Render("q quit · / filter · tab monitor · ←/→ check · r refresh")
	if hi, ok := m.detail(); ok {
		footer = renderDetail(hi)
	}
	// Filter box sits above the grid (Gatus-style). Its height is fixed, so the
	// body always starts at headerRows and checkAt stays valid.
	return header + "\n\n" + m.filterBox() + "\n\n" + body + "\n\n" + footer
}

// filterBox renders the always-present, Gatus-style search box above the grid.
// Gray when idle, accent when focused or holding a query.
func (m model) filterBox() string {
	w := m.width - 2 // account for the box border
	if w < 24 {
		w = 24
	}

	var content string
	style := filterBoxStyle
	switch {
	case m.filtering:
		style = filterBoxActiveStyle
		content = filterLabelStyle.Render("⌕ ") + m.filter.View()
	case m.filter.Value() != "":
		style = filterBoxActiveStyle
		content = filterLabelStyle.Render("⌕ ") + m.filter.Value() +
			subtleStyle.Render("   (/ edit · esc clear)")
	default:
		content = subtleStyle.Render("⌕ press / to filter by name or url")
	}
	return style.Width(w).Render(content)
}

// detail resolves what the panel shows: the mouse hover takes precedence over
// the persistent keyboard selection.
func (m model) detail() (hoverInfo, bool) {
	if m.hover != nil {
		return *m.hover, true
	}
	vis := m.visible()
	if m.sel != nil && m.sel.mon < len(vis) {
		h := vis[m.sel.mon]
		if idx := barStart(h) + m.sel.check; idx >= 0 && idx < len(h.Recent) {
			return hoverInfo{name: h.Name, check: h.Recent[idx]}, true
		}
	}
	return hoverInfo{}, false
}

// barStart is the index of the first displayed bar within Recent (the bar only
// shows the last barLen checks). shownCount is how many bars are displayed.
func barStart(h api.MonitorHealthDTO) int {
	if len(h.Recent) > barLen {
		return len(h.Recent) - barLen
	}
	return 0
}

func shownCount(h api.MonitorHealthDTO) int { return len(h.Recent) - barStart(h) }

func lastCheck(h api.MonitorHealthDTO) int {
	if s := shownCount(h); s > 0 {
		return s - 1
	}
	return 0
}

// checkAt maps a mouse cell (x, y) to the check under it, mirroring the grid
// layout. Returns ok=false when the cell is not over a status bar.
func (m model) checkAt(x, y int) (string, api.CheckDTO, bool) {
	vis := m.visible()
	if len(vis) == 0 {
		return "", api.CheckDTO{}, false
	}
	cols := m.width / cardTotalW
	if cols < 1 {
		cols = 1
	}
	by := y - headerRows
	if by < 0 || by%cardTotalH != barRowInCard {
		return "", api.CheckDTO{}, false // not on a bar row
	}
	gridCol := x / cardTotalW
	if gridCol >= cols {
		return "", api.CheckDTO{}, false
	}
	barIdx := x%cardTotalW - barColStart
	if barIdx < 0 {
		return "", api.CheckDTO{}, false // in the border/padding, not a bar cell
	}
	mi := (by/cardTotalH)*cols + gridCol
	if mi >= len(vis) {
		return "", api.CheckDTO{}, false
	}

	h := vis[mi]
	if barIdx >= shownCount(h) {
		return "", api.CheckDTO{}, false
	}
	return h.Name, h.Recent[barStart(h)+barIdx], true
}

// renderDetail is the hover panel: one line describing a single check.
func renderDetail(hi hoverInfo) string {
	c := hi.check
	state := healthyBadge
	if !c.Up {
		state = unhealthyBadge
	}
	ts := time.Unix(c.At, 0).Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s  %s · code %d · %dms", nameStyle.Render(hi.name), ts, c.StatusCode, c.LatencyMs)
	if stages := renderStages(c); stages != "" {
		line += subtleStyle.Render(stages)
	}
	if c.Error != "" {
		line += " · " + errStyle.Render(c.Error)
	}
	return state + "  " + line
}

// renderStages formats the httptrace breakdown, skipping stages that are zero
// (DNS for an IP, TLS for plain HTTP, etc.).
func renderStages(c api.CheckDTO) string {
	var b strings.Builder
	for _, st := range []struct {
		label string
		ms    int64
	}{
		{"dns", c.DNSMs}, {"conn", c.ConnectMs}, {"tls", c.TLSMs}, {"ttfb", c.TTFBMs},
	} {
		if st.ms > 0 {
			fmt.Fprintf(&b, " · %s %dms", st.label, st.ms)
		}
	}
	return b.String()
}

// arrangeGrid lays cards out in as many columns as fit the terminal width.
func arrangeGrid(cards []string, width int) string {
	cols := width / (cardWidth + 2)
	if cols < 1 {
		cols = 1
	}
	var rows []string
	for i := 0; i < len(cards); i += cols {
		end := i + cols
		if end > len(cards) {
			end = len(cards)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards[i:end]...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func renderCard(h api.MonitorHealthDTO, selIdx int) string {
	selected := selIdx >= 0 // selIdx is -1 only when this is not the active card

	badge := badgeFor(h.State)
	name := nameStyle
	if selected {
		name = selNameStyle
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		name.Render(h.Name),
		"  ",
		badge,
	)

	statLine := fmt.Sprintf("~%dms · uptime %.0f%%", h.LatencyMs, h.UptimePct)
	if h.CertExpiry > 0 {
		days := int(time.Until(time.Unix(h.CertExpiry, 0)).Hours() / 24)
		if days < 0 {
			statLine += " · cert expired"
		} else {
			statLine += fmt.Sprintf(" · cert %dd", days)
		}
	}
	stats := subtleStyle.Render(statLine)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtleStyle.Render(truncate(h.URL, cardWidth)),
		renderBar(h.Recent, selIdx),
		stats,
	)
	style := cardStyle
	if selected {
		style = selectedCardStyle
	}
	return style.Render(body)
}

// badgeFor maps the monitor's wire state to its badge. unknown is its own badge:
// a target that has never been confirmed (fresh start, or failing but still under
// the fail threshold) is not the same as a confirmed down one.
func badgeFor(state string) string {
	switch state {
	case "up":
		return healthyBadge
	case "down":
		return unhealthyBadge
	default:
		return unknownBadge
	}
}

// renderBar draws one colored block per recent check (green up, red down). The
// keyboard-selected bar (selIdx, -1 if none) is a full block; the rest are
// half-blocks that pack tightly like the Gatus bars. Only the most recent bars
// that fit the card are shown.
func renderBar(recent []api.CheckDTO, selIdx int) string {
	if len(recent) == 0 {
		return subtleStyle.Render("no data yet")
	}
	if len(recent) > barLen {
		recent = recent[len(recent)-barLen:]
	}
	var b strings.Builder
	for i, c := range recent {
		glyph := "▌"
		if i == selIdx {
			glyph = "█"
		}
		color := green
		if !c.Up {
			color = red
		}
		b.WriteString(lipgloss.NewStyle().Foreground(color).Render(glyph))
	}
	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func getJSON(c *http.Client, url string, out any) error {
	resp, err := c.Get(url)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---- layout ----
//
// The grid is drawn from these fixed dimensions; checkAt hit-tests against the
// same numbers so a mouse cell maps back to a specific check.
const (
	cardWidth = 40 // card content width (inside the border)
	barLen    = 34 // max bars shown per card (fits cardWidth)

	cardTotalW   = cardWidth + 2 // + left/right border
	cardTotalH   = 6             // top border + 4 content lines + bottom border
	barRowInCard = 3             // border + title + url, then the bar row
	barColStart  = 2             // border + left padding before the first bar cell

	// headerRows is the fixed number of rows above the grid: the title line, a
	// blank, the 3-row filter box, and a blank. Constant (the filter box is
	// always drawn) so checkAt's y-math stays valid.
	headerRows = 6
)

// ---- styles ----

var (
	green  = lipgloss.Color("42")
	red    = lipgloss.Color("196")
	gray   = lipgloss.Color("240")
	fg     = lipgloss.Color("252")
	accent = lipgloss.Color("39") // selected card highlight

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(fg)
	nameStyle    = lipgloss.NewStyle().Bold(true).Foreground(fg)
	selNameStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	subtleStyle  = lipgloss.NewStyle().Foreground(gray)
	helpStyle    = lipgloss.NewStyle().Foreground(gray)
	errStyle     = lipgloss.NewStyle().Bold(true).Foreground(red)

	healthyBadge   = lipgloss.NewStyle().Foreground(green).Render("● healthy")
	unhealthyBadge = lipgloss.NewStyle().Foreground(red).Render("● unhealthy")
	// Gray, not red: unknown means "not confirmed yet", not "down".
	unknownBadge = lipgloss.NewStyle().Foreground(gray).Render("● unknown")

	filterLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)

	filterBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gray).
			Padding(0, 1)

	filterBoxActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accent).
				Padding(0, 1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gray).
			Padding(0, 1).
			Width(cardWidth)

	// selectedCardStyle marks the active card: a thick, accent-colored border.
	// Still a single-cell border, so the grid layout (and hit-test) is unchanged.
	selectedCardStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(accent).
				Padding(0, 1).
				Width(cardWidth)
)
