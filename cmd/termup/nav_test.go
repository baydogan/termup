package main

import (
	"testing"

	"github.com/baydogan/termup/api"
)

func navModel() model {
	return model{
		width: 100,
		health: []api.MonitorHealthDTO{
			{Name: "m0", Recent: recentN(5)},
			{Name: "m1", Recent: recentN(2)},
			{Name: "m2", Recent: recentN(50)}, // shown = barLen
		},
	}
}

func TestNavFirstPressSelectsNewest(t *testing.T) {
	m := navModel()
	m.moveCheck(1) // no prior selection
	if m.sel == nil || m.sel.mon != 0 || m.sel.check != 4 {
		t.Fatalf("sel = %+v, want {0, 4}", m.sel)
	}
}

func TestNavMonitorWrapAndClamp(t *testing.T) {
	m := navModel()
	m.moveMonitor(1) // -> {0, 4}
	m.moveMonitor(1) // -> m1, shown=2, check clamped 4 -> 1
	if m.sel.mon != 1 || m.sel.check != 1 {
		t.Fatalf("sel = %+v, want {1, 1}", m.sel)
	}
	m.moveMonitor(1) // -> m2, shown=34, check 1 stays
	if m.sel.mon != 2 || m.sel.check != 1 {
		t.Fatalf("sel = %+v, want {2, 1}", m.sel)
	}
	m.moveMonitor(1) // wrap -> m0
	if m.sel.mon != 0 {
		t.Fatalf("mon = %d, want 0 (wrap)", m.sel.mon)
	}
	m.moveMonitor(-1) // wrap back -> m2
	if m.sel.mon != 2 {
		t.Fatalf("mon = %d, want 2 (wrap back)", m.sel.mon)
	}
}

func TestNavCheckClamp(t *testing.T) {
	m := navModel()
	m.moveMonitor(1) // {0, 4}
	m.moveCheck(1)   // clamp at shown-1 = 4
	if m.sel.check != 4 {
		t.Fatalf("check = %d, want 4 (clamped high)", m.sel.check)
	}
	m.moveCheck(-100) // clamp at 0
	if m.sel.check != 0 {
		t.Fatalf("check = %d, want 0 (clamped low)", m.sel.check)
	}
}

func TestClampSelectionAfterShrink(t *testing.T) {
	m := navModel()
	m.moveMonitor(1) // {0, 4}
	// Data refresh where m0 shrinks and m2 disappears.
	m.health = []api.MonitorHealthDTO{
		{Name: "m0", Recent: recentN(2)},
	}
	m.clampSelection()
	if m.sel.mon != 0 || m.sel.check != 1 {
		t.Fatalf("sel = %+v, want {0, 1}", m.sel)
	}
	// Selection whose monitor vanished is dropped.
	m.sel = &selRef{mon: 5, check: 0}
	m.clampSelection()
	if m.sel != nil {
		t.Fatalf("sel = %+v, want nil", m.sel)
	}
}
