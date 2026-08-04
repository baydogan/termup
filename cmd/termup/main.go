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

	p := tea.NewProgram(newModel(client, base), tea.WithAltScreen())
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
	client  *http.Client
	base    string
	health  []api.MonitorHealthDTO
	err     error
	updated time.Time
	width   int
	height  int
}

type dataMsg struct {
	health []api.MonitorHealthDTO
	err    error
	at     time.Time
}

type tickMsg time.Time

func newModel(c *http.Client, base string) model {
	return model{client: c, base: base}
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
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			return m, m.fetch
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tea.Batch(m.fetch, tick())
	case dataMsg:
		m.err = msg.err
		if msg.err == nil {
			m.health = msg.health
			m.updated = msg.at
		}
	}
	return m, nil
}

func (m model) View() string {
	header := titleStyle.Render("termup") + "  " +
		subtleStyle.Render("health dashboard")
	if !m.updated.IsZero() {
		header += subtleStyle.Render(" · updated " + m.updated.Format("15:04:05"))
	}

	var body string
	switch {
	case m.err != nil:
		body = errStyle.Render("cannot reach termupd: "+m.err.Error()) +
			subtleStyle.Render("  (is it running?)")
	case len(m.health) == 0:
		body = subtleStyle.Render("loading…")
	default:
		cards := make([]string, len(m.health))
		for i, h := range m.health {
			cards[i] = renderCard(h)
		}
		body = arrangeGrid(cards, m.width)
	}

	footer := helpStyle.Render("q quit · r refresh · auto every 10s")
	return header + "\n\n" + body + "\n\n" + footer
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

func renderCard(h api.MonitorHealthDTO) string {
	badge := healthyBadge
	if h.State != "up" {
		badge = unhealthyBadge
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		nameStyle.Render(h.Name),
		"  ",
		badge,
	)

	stats := subtleStyle.Render(fmt.Sprintf("~%dms · uptime %.0f%%", h.LatencyMs, h.UptimePct))

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtleStyle.Render(truncate(h.URL, cardWidth)),
		renderBar(h.Recent),
		stats,
	)
	return cardStyle.Render(body)
}

// renderBar draws one colored half-block bar per recent check (green up, red
// down). Half-blocks pack tightly yet leave a thin gap, like the Gatus bars.
// Only the most recent bars that fit the card are shown.
func renderBar(recent []api.CheckDTO) string {
	if len(recent) == 0 {
		return subtleStyle.Render("no data yet")
	}
	if len(recent) > barLen {
		recent = recent[len(recent)-barLen:]
	}
	var b strings.Builder
	for _, c := range recent {
		if c.Up {
			b.WriteString(upBar)
		} else {
			b.WriteString(downBar)
		}
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

// ---- styles ----

var (
	green = lipgloss.Color("42")
	red   = lipgloss.Color("196")
	gray  = lipgloss.Color("240")
	fg    = lipgloss.Color("252")

	cardWidth = 40
	barLen    = 34 // max bars shown per card (fits cardWidth)

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(fg)
	nameStyle   = lipgloss.NewStyle().Bold(true).Foreground(fg)
	subtleStyle = lipgloss.NewStyle().Foreground(gray)
	helpStyle   = lipgloss.NewStyle().Foreground(gray)
	errStyle    = lipgloss.NewStyle().Bold(true).Foreground(red)

	healthyBadge   = lipgloss.NewStyle().Foreground(green).Render("● healthy")
	unhealthyBadge = lipgloss.NewStyle().Foreground(red).Render("● unhealthy")

	upBar   = lipgloss.NewStyle().Foreground(green).Render("▌")
	downBar = lipgloss.NewStyle().Foreground(red).Render("▌")

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(gray).
			Padding(0, 1).
			Width(cardWidth)
)
