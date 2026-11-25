package control

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPairingConcurrentConsumeHasExactlyOneWinner(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "pairing-race.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	pairing, code, err := NewPairing(store, now)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, e := pairing.Consume(code, time.Now()); e == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners != 1 {
		t.Fatalf("successful consumes=%d, want exactly 1", winners)
	}
}
