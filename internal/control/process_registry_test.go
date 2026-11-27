package control

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCancelAndWaitSuppressesOrdinaryProcessExitErrors(t *testing.T) {
	registry := NewTaskProcessRegistry()
	done := make(chan struct{})
	close(done)
	result := &processResult{ready: make(chan struct{})}
	result.complete(errors.New("process exited after cancellation"))
	_, err := registry.register("ordinary-exit", func() {}, done, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.CancelAndWait(context.Background(), "ordinary-exit"); err != nil {
		t.Fatalf("ordinary process exit escaped cancellation: %v", err)
	}
}

func TestCancelAndWaitAllowsConcurrentWaitersToObserveResult(t *testing.T) {
	registry := NewTaskProcessRegistry()
	done := make(chan struct{})
	close(done)
	result := &processResult{ready: make(chan struct{})}
	result.complete(ErrCleanupPending)
	releaseCancel := make(chan struct{})
	cancelEntered := make(chan struct{})
	var cancelMu sync.Mutex
	cancelCount := 0
	cancel := func() {
		cancelMu.Lock()
		cancelCount++
		if cancelCount == 2 {
			close(cancelEntered)
		}
		cancelMu.Unlock()
		<-releaseCancel
	}
	unregister, err := registry.register("concurrent-cancel", cancel, done, result)
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()

	start := make(chan struct{})
	responses := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			responses <- registry.CancelAndWait(context.Background(), "concurrent-cancel")
		}()
	}
	close(start)
	select {
	case <-cancelEntered:
	case <-time.After(time.Second):
		t.Fatal("both callers did not reach cancellation")
	}
	close(releaseCancel)

	for range 2 {
		select {
		case err := <-responses:
			if err != ErrCleanupPending {
				t.Fatalf("cancel result=%v, want %v", err, ErrCleanupPending)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent cancellation waiter blocked reading the process result")
		}
	}
}
