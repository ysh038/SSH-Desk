//go:build linux

package devices

import (
	"context"
	"testing"
	"time"
)

// Watch must sleep without events and return promptly when cancelled.
func TestWatchStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	events := make(chan struct{}, 100)
	go func() {
		done <- Watch(ctx, func() {
			select {
			case events <- struct{}{}:
			default:
			}
		})
	}()
	select {
	case err := <-done:
		t.Skipf("cannot watch devices here: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Watch did not stop after cancel")
	}
}
