package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// pollInterval is how often the dashboard re-fetches from termupd. Kept below the
// probe interval so the view stays fresh (probe cadence is the server's).
const pollInterval = 10 * time.Second

type model struct {
	client    *http.Client
	base      string
	health    []api.MonitorHealthDTO
	err       error
	updated   time.Time
	width     int
	height    int
	sel       *selRef
	filter    textinput.Model
	filtering bool // true while the filter input is focused
}

// checkRef is the check the detail panel describes: the one the keyboard
// selection points at.
type checkRef struct {
	name  string
	check api.CheckDTO
}

// selRef is the keyboard selection: a monitor and a displayed bar index within
// it (0-based over the shown window).
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

// detail resolves what the panel shows: the check the keyboard selection points
// at, if any.
func (m model) detail() (checkRef, bool) {
	vis := m.visible()
	if m.sel != nil && m.sel.mon < len(vis) {
		h := vis[m.sel.mon]
		if idx := barStart(h) + m.sel.check; idx >= 0 && idx < len(h.Recent) {
			return checkRef{name: h.Name, check: h.Recent[idx]}, true
		}
	}
	return checkRef{}, false
}
