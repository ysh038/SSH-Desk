package mypc

import (
	"context"
	"sync"
	"time"

	"tuidesk/internal/devices"
	"tuidesk/internal/fsops"
	"tuidesk/internal/host"
	"tuidesk/internal/term"
)

// Monitor tells the user when a removable drive is connected or
// disconnected and keeps open My PC windows current (SPEC 7.8 hot-plug).
// It does nothing by itself: devices.Watch calls Trigger when the kernel
// reports a change.
type Monitor struct {
	h   host.Host
	src devices.Source
	// Delay collects a burst of events (a disk and its partitions, udev's
	// follow-ups) into one device scan.
	Delay time.Duration

	mu    sync.Mutex
	timer *time.Timer

	// UI goroutine only.
	known  map[string]string
	stale  map[string]bool // folders held by removed drives, already announced
	primed bool
	gen    int
}

// NewMonitor returns a monitor; call Check once to learn what is already
// connected.
func NewMonitor(h host.Host, src devices.Source) *Monitor {
	return &Monitor{h: h, src: src, Delay: time.Second}
}

// Trigger schedules a Check after Delay, restarting the wait on every
// call. Safe from any goroutine.
func (m *Monitor) Trigger() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.Delay, func() { m.h.Post(m.Check) })
}

// Check scans the devices now, announces removable drives that appeared or
// disappeared since the last scan and refreshes My PC windows. The first
// scan announces nothing. UI goroutine only.
func (m *Monitor) Check() {
	m.gen++
	gen := m.gen
	var vols []devices.Volume
	m.h.Go("Reading devices", func(ctx context.Context, _ func(fsops.Progress)) error {
		var err error
		vols, err = m.src.List(ctx)
		return err
	}, func(err error) {
		if err != nil || gen != m.gen {
			return
		}
		cur := devices.RemovableDisks(vols)
		stale := map[string]bool{}
		for _, dir := range devices.StaleDirs(vols) {
			stale[dir] = true
		}
		if m.primed && m.h.Config().NotifyDrives {
			added, removed := devices.DiffDisks(m.known, cur)
			for _, n := range added {
				m.h.Notify("USB drive connected: "+m.display(n)+" - open My PC to use it", false)
			}
			for _, n := range removed {
				m.h.Notify("Drive disconnected: "+m.display(n), false)
			}
		}
		if m.primed {
			// Always said, whatever the notification setting: files may be
			// lost and the folder stays blocked until it is cleaned up.
			for dir := range stale {
				if !m.stale[dir] {
					m.h.Notify("Removed without Eject - open My PC to clean up "+m.display(dir), true)
				}
			}
		}
		m.known, m.stale, m.primed = cur, stale, true
		for _, app := range m.h.Apps() {
			if a, ok := app.(*App); ok {
				a.Refresh()
			}
		}
	})
}

func (m *Monitor) display(s string) string {
	if m.h.Console() {
		return term.EscapeNonASCII(s)
	}
	return s
}
