package batch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu      sync.Mutex
	batches [][]int
	err     error
}

func (r *recorder) flush(items []int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, items)
	return r.err
}

func (r *recorder) snapshot() [][]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]int(nil), r.batches...)
}

func TestFlushesWhenFull(t *testing.T) {
	rec := &recorder{}
	b := New(3, time.Hour, rec.flush)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := b.Add(context.Background(), i); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	b.Close()

	got := rec.snapshot()
	if len(got) != 2 || len(got[0]) != 3 || len(got[1]) != 3 {
		t.Fatalf("batches = %v, want two full batches of 3", got)
	}
}

func TestFlushesOnInterval(t *testing.T) {
	rec := &recorder{}
	b := New(100, 20*time.Millisecond, rec.flush)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.Add(ctx, 1); err != nil {
		t.Fatalf("a partial batch was not flushed by the interval: %v", err)
	}
	if len(rec.snapshot()) != 1 {
		t.Fatal("Add returned before its batch was flushed")
	}
}

func TestAddReturnsTheFlushError(t *testing.T) {
	rec := &recorder{err: errors.New("database unavailable")}
	b := New(1, time.Hour, rec.flush)
	defer b.Close()

	if err := b.Add(context.Background(), 1); err == nil || err.Error() != "database unavailable" {
		t.Fatalf("Add = %v, want the flush error", err)
	}
}

func TestAddRespectsContextWhileWaiting(t *testing.T) {
	block := make(chan struct{})
	b := New(1, time.Hour, func([]int) error { <-block; return nil })
	defer func() { close(block); b.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := b.Add(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Add = %v, want the context error while the flush is blocked", err)
	}
}

func TestCloseFlushesBufferedItems(t *testing.T) {
	rec := &recorder{}
	b := New(100, time.Hour, rec.flush)
	done := make(chan error, 1)
	go func() { done <- b.Add(context.Background(), 7) }()
	// Wait until the item is buffered, then close.
	for len(b.items) > 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	b.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := rec.snapshot(); len(got) != 1 || got[0][0] != 7 {
		t.Fatalf("batches = %v, want [[7]]", got)
	}
}
