package notify

import "log"

var _ Notifier = (*Multi)(nil)

// Multi fans an event out to several notifiers in order. One child's failure is
// logged and does not stop the others, so Notify itself never returns an error.
type Multi struct {
	children []Notifier
}

func NewMulti(children ...Notifier) *Multi {
	return &Multi{children: children}
}

func (m *Multi) Notify(e Event) error {
	for _, c := range m.children {
		if err := c.Notify(e); err != nil {
			log.Printf("notify: %v", err)
		}
	}
	return nil
}
