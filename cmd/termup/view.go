package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/baydogan/termup/api"
	"github.com/baydogan/termup/cmd/termup/style"
	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	return strings.Join(append(m.headerBlocks(), m.body(), "", m.footer()), "\n")
}

// headerBlocks are the blocks drawn above the grid, in order: the title line, a
// blank row, the always-present filter box, another blank row.
func (m model) headerBlocks() []string {
	return []string{m.headerLine(), "", m.filterBox(), ""}
}

func (m model) headerLine() string {
	line := style.Title.Render("termup") + "  " + style.Subtle.Render("health dashboard")
	if !m.updated.IsZero() {
		line += style.Subtle.Render(" · updated " + m.updated.Format("15:04:05"))
	}
	return line
}

// body is the grid, or the message that replaces it.
func (m model) body() string {
	vis := m.visible()
	switch {
	case m.err != nil:
		return style.Err.Render("cannot reach termupd: "+m.err.Error()) +
			style.Subtle.Render("  (is it running?)")
	case len(m.health) == 0:
		return style.Subtle.Render("loading…")
	case len(vis) == 0:
		return style.Subtle.Render("no monitors match the filter")
	}
	cards := make([]string, len(vis))
	for i, h := range vis {
		sel := -1
		if m.sel != nil && m.sel.mon == i {
			sel = m.sel.check
		}
		cards[i] = renderCard(h, sel)
	}
	return arrangeGrid(cards, m.width)
}

// footer is the key help, replaced by the detail panel while a check is selected.
func (m model) footer() string {
	if hi, ok := m.detail(); ok {
		return renderDetail(hi)
	}
	return style.Help.Render("q quit · / filter · tab monitor · ←/→ check · r refresh")
}

// filterBox renders the always-present, Gatus-style search box above the grid.
func (m model) filterBox() string {
	w := m.width - 2 // account for the box border
	if w < 24 {
		w = 24
	}

	var content string
	boxStyle := style.FilterBox
	switch {
	case m.filtering:
		boxStyle = style.FilterBoxActive
		content = style.FilterLabel.Render("⌕ ") + m.filter.View()
	case m.filter.Value() != "":
		boxStyle = style.FilterBoxActive
		content = style.FilterLabel.Render("⌕ ") + m.filter.Value() +
			style.Subtle.Render("   (/ edit · esc clear)")
	default:
		content = style.Subtle.Render("⌕ press / to filter by name or url")
	}
	return boxStyle.Width(w).Render(content)
}

func renderCard(h api.MonitorHealthDTO, selIdx int) string {
	selected := selIdx >= 0 // selIdx is -1 only when this is not the active card

	name := style.Name
	cardStyle := style.Card
	if selected {
		name = style.SelName
		cardStyle = style.SelectedCard
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		name.Render(h.Name),
		"  ",
		badgeFor(h.State),
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

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		style.Subtle.Render(truncate(h.URL, cardWidth)),
		"", // breathing room: the url sits right on top of the bars otherwise
		renderBar(h.Recent, selIdx),
		renderMarker(h.Recent, selIdx),
		style.Subtle.Render(statLine),
	)
	return cardStyle.Width(cardWidth).Render(body)
}

// renderMarker is the row under the bar that points at the selected check. It is
// always drawn, even with nothing selected, so every card in a grid row keeps the
// same height (JoinHorizontal would otherwise pad them apart).
func renderMarker(recent []api.CheckDTO, selIdx int) string {
	shown := len(recent)
	if shown > barLen {
		shown = barLen
	}
	if selIdx < 0 || selIdx >= shown {
		return " "
	}
	return strings.Repeat(" ", selIdx) + style.Marker.Render(markerGlyph)
}

// badgeFor maps the monitor's wire state to its badge. unknown is its own badge:
// a target that has never been confirmed (fresh start, or failing but still under
// the fail threshold) is not the same as a confirmed down one.
func badgeFor(state string) string {
	switch state {
	case "up":
		return style.HealthyBadge
	case "down":
		return style.UnhealthyBadge
	default:
		return style.UnknownBadge
	}
}

// barGlyph is a left half block: consecutive ones pack tightly with a gap, like
// the Gatus bars. Every bar uses it, selected or not — see barColor.
const barGlyph = "▌"

// renderBar draws one colored block per recent check (green up, red down), with
// the keyboard-selected bar (selIdx, -1 if none) brightened. Only the most recent
// bars that fit the card are shown.
func renderBar(recent []api.CheckDTO, selIdx int) string {
	if len(recent) == 0 {
		return style.Subtle.Render("no data yet")
	}
	if len(recent) > barLen {
		recent = recent[len(recent)-barLen:]
	}
	var b strings.Builder
	for i, c := range recent {
		b.WriteString(barStyle(c.Up, i == selIdx).Render(barGlyph))
	}
	return b.String()
}

// markerGlyph points at the selected bar from the row below it.
const markerGlyph = "▲"

// barStyle is how one bar is painted. The selected cell keeps the glyph and gains
// a brighter colour plus a tinted background; the pointer in the marker row is
// what actually identifies it. Bold and a wider glyph were both tried first: a
// solid block has no stroke weight to thicken, and ▌→█ fills the other half of
// the cell, which reads as the bar sliding sideways.
func barStyle(up, selected bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(barColor(up, selected))
	if selected {
		s = s.Background(style.BarSelBg)
	}
	return s
}

// barColor marks selection with light, not geometry: the glyph is the same in
// every state, so the bar cannot appear to move when the selection lands on it.
func barColor(up, selected bool) lipgloss.Color {
	switch {
	case up && selected:
		return style.GreenBright
	case up:
		return style.Green
	case selected:
		return style.RedBright
	default:
		return style.Red
	}
}

// renderDetail is the panel under the grid: one line describing the selected check.
func renderDetail(hi checkRef) string {
	c := hi.check
	badge := style.HealthyBadge
	if !c.Up {
		badge = style.UnhealthyBadge
	}
	ts := time.Unix(c.At, 0).Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s  %s · code %d · %dms", style.Name.Render(hi.name), ts, c.StatusCode, c.LatencyMs)
	if stages := renderStages(c); stages != "" {
		line += style.Subtle.Render(stages)
	}
	if c.Error != "" {
		line += " · " + style.Err.Render(c.Error)
	}
	return badge + "  " + line
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
