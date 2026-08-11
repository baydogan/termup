package notify_test

import (
	"errors"
	"testing"

	"github.com/baydogan/termup/notify"
)

type countingNotifier struct {
	calls int
	err   error
}

func (c *countingNotifier) Notify(notify.Event) error {
	c.calls++
	return c.err
}

func TestMultiFansOutDespiteError(t *testing.T) {
	a := &countingNotifier{err: errors.New("boom")} // failing child
	b := &countingNotifier{}
	c := &countingNotifier{}

	m := notify.NewMulti(a, b, c)
	if err := m.Notify(notify.Event{Monitor: "x"}); err != nil {
		t.Fatalf("Multi.Notify returned %v, want nil", err)
	}

	// Every child is called even though the first one failed.
	for i, n := range []*countingNotifier{a, b, c} {
		if n.calls != 1 {
			t.Errorf("child %d called %d times, want 1", i, n.calls)
		}
	}
}
