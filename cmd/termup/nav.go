package main

// Keyboard selection. Every move is expressed over the visible (filtered) list,
// so the indices always match what is on screen.

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
